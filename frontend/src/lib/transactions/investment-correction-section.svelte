<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import AlertTriangle from '@lucide/svelte/icons/triangle-alert';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import CapitalReturnForm from '#lib/investments/capital-return-form.svelte';
  import CashInLieuForm from '#lib/investments/cash-in-lieu-form.svelte';
  import BuyForm from '#lib/investments/buy-form.svelte';
  import SellForm from '#lib/investments/sell-form.svelte';
  import SplitForm from '#lib/investments/split-form.svelte';
  import DividendCorrectionForm from '#lib/investments/dividend-correction-form.svelte';
  import WriteOffCorrectionForm from '#lib/investments/write-off-correction-form.svelte';
  import TransferCorrectionForm from '#lib/investments/transfer-correction-form.svelte';
  import TransferInCorrectionForm from '#lib/investments/transfer-in-correction-form.svelte';
  import BasisResolutionForm from '#lib/investments/basis-resolution-form.svelte';
  import ShortTradeForm from '#lib/investments/short-trade-form.svelte';
  import ShareExchangeSummary from '#lib/investments/share-exchange-summary.svelte';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import GainImpactList from '#lib/investments/gain-impact-list.svelte';
  import { invalidateInvestmentReads } from '#lib/investments/invalidate.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    gainAcknowledgement,
    gainImpactCurrency,
    gainImpactRows,
    hasGainChanges,
    impactNeedsReview,
    isGainAcknowledgementRefusal
  } from '#lib/investments/gain-impact.ts';
  import {
    getInvestmentCorrectionChain,
    getInvestmentTradeCorrectionContext,
    investmentInstrumentsQueryOptions,
    investmentCorrectionChainQueryKey,
    previewBuyReversalReconciliation,
    previewDividendReversalReconciliation,
    previewReinvestmentReversalReconciliation,
    previewSaleReversalReconciliation,
    previewSplitReversalReconciliation,
    previewTransferReversalReconciliation,
    previewWriteOffReversalReconciliation,
    reverseDividend,
    reverseCapitalReturn, reverseCashInLieu, previewCashInLieuReversalReconciliation,
    previewCapitalReturnReversalReconciliation,
    reverseManualBuy,
    reverseReinvestedDividend,
    reverseManualSale,
    reverseSplit,
    reverseTransfer,
    reverseWriteOff,
    previewShortSaleReversal,
    reverseShortSale,
    previewShortCoverReversal,
    reverseShortCover,
    previewBasisResolutionReversal,
    reverseBasisResolution,
    type GainImpact,
    type ReconciliationImpactResponse
  } from '#lib/api/investments.ts';
  import { formatQuantity } from '#lib/money/format.ts';

  let {
    transactionID,
    csrfToken,
    onRefresh,
    systemLabel
  }: {
    transactionID: number;
    csrfToken?: string;
    onRefresh?: () => void;
    // A split adjustment journal shows the chain of the split it adjusts (T-136).
    systemLabel?: string;
  } = $props();

  const queryClient = useQueryClient();

  const chainQuery = createQuery(() => ({
    queryKey: [...investmentCorrectionChainQueryKey, transactionID],
    queryFn: () => getInvestmentCorrectionChain(transactionID),
    enabled: transactionID > 0
  }));

  let replacementKind = $state<'buy' | 'sell' | 'split' | 'dividend' | 'reinvestment' | 'write_off' | 'transfer' | 'capital_return' | 'cash_in_lieu' | 'cash_in_lieu_entry' | 'resolve_basis' | 'correct_basis' | 'short_sale' | 'short_cover' | null>(null);
  // Splits pre-fill from the chain's effective_split; only trades need the
  // separate source-facts read.
  const replacementQuery = createQuery(() => ({
    queryKey: [...investmentCorrectionChainQueryKey, 'source', transactionID],
    queryFn: () => getInvestmentTradeCorrectionContext(transactionID),
    enabled: (replacementKind === 'buy' || replacementKind === 'sell' || replacementKind === 'write_off' || replacementKind === 'cash_in_lieu' ||
      replacementKind === 'short_sale' || replacementKind === 'short_cover') && transactionID > 0
  }));
  const splitCorrectable = $derived(chainQuery.data?.can_correct_split === true &&
    chainQuery.data.effective_transaction_id === transactionID && !!chainQuery.data.effective_split);

  // Dividends and reinvestments pre-fill from the chain's effective terms (T-115).
  const cashInLieuCorrectable = $derived(chainQuery.data?.can_correct_cash_in_lieu === true && chainQuery.data.effective_transaction_id === transactionID);
  const capitalReturnReversible = $derived(chainQuery.data?.can_reverse_return_of_capital === true &&
    chainQuery.data.effective_transaction_id === transactionID);
  const dividendCorrectable = $derived(chainQuery.data?.can_correct_dividend === true &&
    chainQuery.data.effective_transaction_id === transactionID && !!chainQuery.data.effective_dividend);
  const reinvestmentCorrectable = $derived(chainQuery.data?.can_correct_reinvested_dividend === true &&
    chainQuery.data.effective_transaction_id === transactionID && !!chainQuery.data.effective_reinvestment);

  // Write-offs pre-fill from the trade correction context (T-118).
  const writeOffCorrectable = $derived(chainQuery.data?.can_correct_write_off === true &&
    chainQuery.data.effective_transaction_id === transactionID);

  // Named short sales and covers are reversed or corrected natively (#175);
  // the correction form pre-fills from the trade correction context.
  const shortKind = $derived(chainQuery.data?.effective_transaction_id !== transactionID ? null
    : chainQuery.data?.can_correct_short_sale ? 'short_sale' as const
      : chainQuery.data?.can_correct_short_cover ? 'short_cover' as const : null);

  // Internal and external-in transfers are reversed or replaced; the
  // replacement form pre-fills from the chain's effective_transfer (T-119).
  const transferReversible = $derived(chainQuery.data?.can_reverse_transfer === true &&
    chainQuery.data.effective_transaction_id === transactionID);
  const transferReplaceable = $derived(chainQuery.data?.can_replace_transfer === true &&
    chainQuery.data.effective_transaction_id === transactionID && !!chainQuery.data.effective_transfer);
  // An external transfer in recorded with unknown basis can be resolved from
  // a sourced statement (T-145).
  const basisResolvable = $derived(chainQuery.data?.can_resolve_basis === true &&
    chainQuery.data.effective_transaction_id === transactionID && !!chainQuery.data.effective_transfer);
  // Its effective resolution is corrected or withdrawn through the transfer;
  // every resolution stays listed with its standing (#168).
  const basisCorrectable = $derived(chainQuery.data?.can_correct_basis_resolution === true &&
    chainQuery.data.effective_transaction_id === transactionID && !!chainQuery.data.effective_basis_resolution);
  const basisResolutions = $derived(chainQuery.data?.effective_transaction_id === transactionID
    ? chainQuery.data.basis_resolutions : []);
  // Every resolution of a transfer is in its cost currency: the effective
  // terms name it, else the transfer's own terms.
  const basisCurrencyCode = $derived.by(() => {
    const currencyID = chainQuery.data?.effective_basis_resolution?.cost_commodity_id
      ?? chainQuery.data?.effective_transfer?.cost_commodity_id;
    return (currenciesQuery.data?.currencies ?? []).find((c: CurrencyResponse) => c.id === currencyID)?.code ?? '';
  });

  // A share exchange is explained with its instruments, holdings, ratio and
  // carried basis (#178); it is not yet correctable (#179).
  const shareExchange = $derived(chainQuery.data?.effective_transaction_id === transactionID
    ? chainQuery.data.effective_share_exchange ?? null : null);
  const instrumentsQuery = createQuery(() => ({ ...investmentInstrumentsQueryOptions(), enabled: shareExchange !== null }));
  const exchangeAccountsQuery = createQuery(() => ({ ...accountsQueryOptions(false, false), enabled: shareExchange !== null }));
  function exchangeInstrumentName(commodityID: number): string {
    const instrument = (instrumentsQuery.data?.instruments ?? []).find((item) => item.commodity_id === commodityID);
    return instrument?.display_name ?? instrument?.commodity_code ?? `#${commodityID}`;
  }
  function exchangeAccountName(accountID: number): string {
    return (exchangeAccountsQuery.data?.accounts ?? []).find((account) => account.id === accountID)?.name ?? `#${accountID}`;
  }

  const sourceLinkedEffectiveBuy = $derived(
    chainQuery.data?.can_reverse_buy === true && chainQuery.data.effective_transaction_id === transactionID
  );

  let modal = $state<'closed' | 'reason' | 'reconciliation'>('closed');
  let reversalKind = $state<'buy' | 'sale' | 'split' | 'dividend' | 'reinvestment' | 'write_off' | 'transfer' | 'capital_return' | 'cash_in_lieu' | 'short_sale' | 'short_cover' | 'basis_resolution'>('sale');
  let reason = $state('');
  let pending = $state(false);
  let actionError = $state<unknown>(undefined);
  let impacts = $state<ReconciliationImpactResponse['affected_checkpoints']>([]);
  // A reversal removes the reversed sale's gain and can revise later sales
  // (T-126). The review step lists those beside any checkpoints.
  let gainImpact = $state<GainImpact | null>(null);
  let gainRefreshed = $state(false);
  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  const gainRows = $derived(gainImpact
    ? gainImpactRows(gainImpact.changes, gainImpactCurrency(new Map(
        (currenciesQuery.data?.currencies ?? []).map((c: CurrencyResponse) => [c.id, c]))), getLocale())
    : []);
  let reasonInputElement: HTMLInputElement | undefined = $state();
  let confirmButtonElement: HTMLButtonElement | undefined = $state();

  $effect(() => {
    if (modal === 'reason') reasonInputElement?.focus();
    if (modal === 'reconciliation') confirmButtonElement?.focus();
  });

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && modal !== 'closed' && !pending) closeModal();
    else if (event.key === 'Escape' && replacementKind !== null) replacementKind = null;
  }

  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale, {
    year: 'numeric', month: 'short', day: 'numeric'
  }));

  function formatDate(value: string): string {
    return dateFormatter.format(parseISO(value));
  }

  function closeModal() {
    modal = 'closed';
    actionError = undefined;
    impacts = [];
    gainImpact = null;
    gainRefreshed = false;
  }

  // Preview through the actual reversal writer; any checkpoint or gain
  // consequence moves to the review step instead of committing.
  async function previewReversal(refreshed: boolean): Promise<boolean> {
    const body = { reason: reason.trim() };
    const preview = reversalKind === 'buy'
      ? await previewBuyReversalReconciliation(transactionID, body)
      : reversalKind === 'split'
        ? await previewSplitReversalReconciliation(transactionID, body)
        : reversalKind === 'dividend'
          ? await previewDividendReversalReconciliation(transactionID, body)
          : reversalKind === 'reinvestment'
            ? await previewReinvestmentReversalReconciliation(transactionID, body)
            : reversalKind === 'write_off'
              ? await previewWriteOffReversalReconciliation(transactionID, body)
              : reversalKind === 'transfer'
                ? await previewTransferReversalReconciliation(transactionID, body)
                : reversalKind === 'capital_return'
                  ? await previewCapitalReturnReversalReconciliation(transactionID, body)
                  : reversalKind === 'cash_in_lieu'
                    ? await previewCashInLieuReversalReconciliation(transactionID, body)
                    : reversalKind === 'short_sale'
                      ? await previewShortSaleReversal(transactionID, body)
                      : reversalKind === 'short_cover'
                        ? await previewShortCoverReversal(transactionID, body)
                        : reversalKind === 'basis_resolution'
                          ? await previewBasisResolutionReversal(transactionID, body)
                          : await previewSaleReversalReconciliation(transactionID, body);
    if (!impactNeedsReview(preview)) return false;
    impacts = preview.affected_checkpoints;
    gainImpact = hasGainChanges(preview.gain_impact) ? preview.gain_impact : null;
    gainRefreshed = refreshed && gainImpact !== null;
    modal = 'reconciliation';
    return true;
  }

  async function commitReversal() {
    if (!csrfToken) return;
    const acknowledgement = gainAcknowledgement(gainImpact);
    const body = {
      reason: reason.trim(),
      ...(impacts.length > 0 ? { reconciliation_override: true } : {}),
      ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {})
    };
    if (reversalKind === 'buy') {
      await reverseManualBuy(transactionID, body, csrfToken);
    } else if (reversalKind === 'split') {
      await reverseSplit(transactionID, body, csrfToken);
    } else if (reversalKind === 'dividend') {
      await reverseDividend(transactionID, body, csrfToken);
    } else if (reversalKind === 'reinvestment') {
      await reverseReinvestedDividend(transactionID, body, csrfToken);
    } else if (reversalKind === 'write_off') {
      await reverseWriteOff(transactionID, body, csrfToken);
    } else if (reversalKind === 'transfer') {
      await reverseTransfer(transactionID, body, csrfToken);
    } else if (reversalKind === 'capital_return') {
      await reverseCapitalReturn(transactionID, body, csrfToken);
    } else if (reversalKind === 'cash_in_lieu') {
      await reverseCashInLieu(transactionID, body, csrfToken);
    } else if (reversalKind === 'short_sale') {
      await reverseShortSale(transactionID, body, csrfToken);
    } else if (reversalKind === 'short_cover') {
      await reverseShortCover(transactionID, body, csrfToken);
    } else if (reversalKind === 'basis_resolution') {
      await reverseBasisResolution(transactionID, body, csrfToken);
    } else {
      await reverseManualSale(transactionID, body, csrfToken);
    }
    // A reversal restates positions, lots and realized gains, not only the
    // transaction lists the parent refreshes (T-138).
    await invalidateInvestmentReads(queryClient);
    closeModal();
    onRefresh?.();
  }

  async function submitReason() {
    if (!csrfToken || !reason.trim()) return;
    pending = true;
    actionError = undefined;
    try {
      if (!(await previewReversal(false))) await commitReversal();
    } catch (error) {
      actionError = error;
      modal = 'reason';
    } finally {
      pending = false;
    }
  }

  async function confirmReconciliation() {
    if (!csrfToken || !reason.trim()) return;
    pending = true;
    actionError = undefined;
    try {
      await commitReversal();
    } catch (error) {
      // The gain set changed since review: show the current one, not a dead end.
      try {
        if (!isGainAcknowledgementRefusal(error) || !(await previewReversal(true))) actionError = error;
      } catch (previewError) {
        actionError = previewError;
      }
    } finally {
      pending = false;
    }
  }

  function replacementSaved() {
    replacementKind = null;
    void chainQuery.refetch();
    onRefresh?.();
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<section class="space-y-3 border-t border-border pt-4" aria-labelledby="investment-correction-heading">
  <h3 id="investment-correction-heading" class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">
    {m.transactions_investment_history_title()}
  </h3>

  {#if chainQuery.isPending}
    <p class="text-sm text-muted" role="status">{m.transactions_investment_history_loading()}</p>
  {:else if chainQuery.isError}
    <div class="space-y-2">
      <APIFormError error={chainQuery.error} />
      <button type="button" class="text-sm font-semibold text-accent" onclick={() => chainQuery.refetch()}>
        {m.transactions_retry()}
      </button>
    </div>
  {:else if chainQuery.data && chainQuery.data.operations.length > 0}
    <ol class="space-y-2">
      {#each chainQuery.data.operations as node, index (node.operation_id)}
        <li class="rounded-[var(--radius-control)] border border-border bg-surface px-3 py-2 text-sm">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <span class="font-medium text-foreground">
              {#if index === 0}
                {m.transactions_investment_history_original()}
              {:else if node.correction_mode === 'reverse'}
                {m.transactions_investment_history_reversal()}
              {:else}
                {m.transactions_investment_history_replacement()}
              {/if}
              {#if node.transaction_id} · #{node.transaction_id}{/if}
            </span>
            <span class="text-xs text-muted">
              {#if node.effective}
                {m.transactions_investment_history_current()}
              {:else if node.correction_mode === 'reverse' || chainQuery.data.operations.at(-1)?.correction_mode === 'reverse'}
                {m.transactions_investment_history_reversed()}
              {:else}
                {m.transactions_investment_history_superseded()}
              {/if}
            </span>
          </div>
          <p class="mt-1 text-xs text-muted">{formatDate(node.event_date)}</p>
          {#if node.correction_reason}
            <p class="mt-1 text-xs text-foreground">{node.correction_reason}</p>
          {/if}
        </li>
      {/each}
    </ol>
    {#if chainQuery.data.operations.some((node) => node.imported)}
      <p class="text-xs text-muted">{m.transactions_investment_history_imported()}</p>
    {/if}
    {#if basisResolutions.length > 0}
      <div class="space-y-2" role="group" aria-labelledby="basis-resolution-history-heading">
        <h4 id="basis-resolution-history-heading" class="text-xs font-semibold text-muted">
          {m.investments_basis_resolution_history_title()}
        </h4>
        <ol class="space-y-2">
          {#each basisResolutions as entry (entry.operation_id)}
            <li class="rounded-[var(--radius-control)] border border-border bg-surface px-3 py-2 text-sm">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <span class="font-medium text-foreground">
                  {m.investments_basis_resolution_history_amount({
                    amount: formatQuantity(entry.basis_value, entry.basis_scale, locale),
                    currency: basisCurrencyCode
                  })}
                  {#if entry.transaction_id} · #{entry.transaction_id}{/if}
                </span>
                <span class="text-xs text-muted">
                  {entry.status === 'effective' ? m.transactions_investment_history_current()
                    : entry.status === 'reversed' ? m.transactions_investment_history_reversed()
                    : m.transactions_investment_history_superseded()}
                </span>
              </div>
              <p class="mt-1 text-xs text-muted">
                {m.investments_basis_resolution_history_recorded({ date: formatDate(entry.created_at) })}
                {#if entry.transaction_id === null} · {m.investments_basis_resolution_history_no_journal()}{/if}
              </p>
              {#if entry.reason}
                <p class="mt-1 text-xs text-foreground">{entry.reason}</p>
              {/if}
              {#if entry.reversal_reason}
                <p class="mt-1 text-xs text-foreground">
                  {m.investments_basis_resolution_history_withdrawn({ reason: entry.reversal_reason })}
                </p>
              {/if}
            </li>
          {/each}
        </ol>
      </div>
    {/if}
    {#if shareExchange}
      <div class="space-y-2" role="group" aria-labelledby="share-exchange-detail-heading">
        <h4 id="share-exchange-detail-heading" class="text-xs font-semibold text-muted">{m.transactions_investment_exchange_title()}</h4>
        {#if instrumentsQuery.isPending || exchangeAccountsQuery.isPending}
          <p class="text-sm text-muted" role="status">{m.transactions_investment_history_loading()}</p>
        {:else}
          <p class="text-sm text-foreground">{m.transactions_investment_exchange_summary({
            oldName: exchangeInstrumentName(shareExchange.commodity_id),
            newName: exchangeInstrumentName(shareExchange.destination_commodity_id),
            ratioNew: String(shareExchange.plan.ratio_numerator), ratioOld: String(shareExchange.plan.ratio_denominator),
            date: formatDate(shareExchange.effective_on)
          })}</p>
          <p class="text-xs text-muted">{shareExchange.destination_holding_account_id === shareExchange.holding_account_id
            ? m.transactions_investment_exchange_same_holding({ name: exchangeAccountName(shareExchange.holding_account_id) })
            : m.transactions_investment_exchange_holdings({
              source: exchangeAccountName(shareExchange.holding_account_id),
              destination: exchangeAccountName(shareExchange.destination_holding_account_id)
            })}</p>
          <ShareExchangeSummary plan={shareExchange.plan}
            oldName={exchangeInstrumentName(shareExchange.commodity_id)}
            newName={exchangeInstrumentName(shareExchange.destination_commodity_id)}
            currenciesByID={new Map((currenciesQuery.data?.currencies ?? []).map((c: CurrencyResponse) => [c.id, c]))} />
        {/if}
        <p class="text-xs text-muted" role="note">{m.transactions_investment_exchange_not_correctable()}</p>
      </div>
    {/if}
    {#if systemLabel === 'split_adjustment'}
      <p class="text-xs text-muted">{m.transactions_investment_split_adjustment_note()}</p>
    {/if}
    {#if splitCorrectable}
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
          disabled={!csrfToken || pending}
          onclick={() => { reversalKind = 'split'; reason = ''; actionError = undefined; modal = 'reason'; }}
        >
          {m.transactions_investment_reverse_split_action()}
        </button>
        <button type="button"
          class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
          disabled={!csrfToken}
          onclick={() => { replacementKind = 'split'; }}>
          {m.transactions_investment_replace_split_action()}
        </button>
        <button type="button" disabled={!csrfToken} onclick={() => (replacementKind = 'cash_in_lieu_entry')}
          class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground disabled:opacity-60">{m.investments_cash_in_lieu_entry_action()}</button>
      </div>
    {/if}
    {#if cashInLieuCorrectable}
      <div class="flex flex-wrap gap-2">
        <button type="button" disabled={!csrfToken || pending} onclick={() => { reversalKind = 'cash_in_lieu'; reason = ''; actionError = undefined; modal = 'reason'; }}
          class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground disabled:opacity-60">{m.investments_cash_in_lieu_reverse_action()}</button>
        <button type="button" disabled={!csrfToken} onclick={() => (replacementKind = 'cash_in_lieu')}
          class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground disabled:opacity-60">{m.investments_cash_in_lieu_correct()}</button>
      </div>
    {/if}
    {#if capitalReturnReversible}
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
          disabled={!csrfToken || pending}
          onclick={() => { reversalKind = 'capital_return'; reason = ''; actionError = undefined; modal = 'reason'; }}
        >
          {m.transactions_investment_reverse_capital_return_action()}
        </button>
        {#if chainQuery.data?.can_replace_return_of_capital && chainQuery.data.effective_return_of_capital}
          <button type="button" disabled={!csrfToken}
            class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
            onclick={() => (replacementKind = 'capital_return')}>{m.investments_capital_return_correct()}</button>
        {/if}
      </div>
    {/if}
    {#if dividendCorrectable || reinvestmentCorrectable}
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
          disabled={!csrfToken || pending}
          onclick={() => { reversalKind = dividendCorrectable ? 'dividend' : 'reinvestment'; reason = ''; actionError = undefined; modal = 'reason'; }}
        >
          {dividendCorrectable ? m.transactions_investment_reverse_dividend_action() : m.transactions_investment_reverse_reinvestment_action()}
        </button>
        <button type="button"
          class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
          disabled={!csrfToken}
          onclick={() => { replacementKind = dividendCorrectable ? 'dividend' : 'reinvestment'; }}>
          {dividendCorrectable ? m.transactions_investment_replace_dividend_action() : m.transactions_investment_replace_reinvestment_action()}
        </button>
      </div>
    {/if}
    {#if writeOffCorrectable}
      <div class="flex flex-wrap gap-2">
        <button
          type="button"
          class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
          disabled={!csrfToken || pending}
          onclick={() => { reversalKind = 'write_off'; reason = ''; actionError = undefined; modal = 'reason'; }}
        >
          {m.transactions_investment_reverse_write_off_action()}
        </button>
        <button type="button"
          class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
          disabled={!csrfToken}
          onclick={() => { replacementKind = 'write_off'; }}>
          {m.transactions_investment_replace_write_off_action()}
        </button>
      </div>
    {/if}
    {#if shortKind}
      <div class="flex flex-wrap gap-2">
        <button type="button"
          class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
          disabled={!csrfToken || pending}
          onclick={() => { reversalKind = shortKind ?? 'short_sale'; reason = ''; actionError = undefined; modal = 'reason'; }}>
          {shortKind === 'short_sale' ? m.investments_short_sale_reverse_action() : m.investments_short_cover_reverse_action()}
        </button>
        <button type="button"
          class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
          disabled={!csrfToken}
          onclick={() => { replacementKind = shortKind; }}>
          {shortKind === 'short_sale' ? m.investments_short_sale_correct_action() : m.investments_short_cover_correct_action()}
        </button>
      </div>
    {/if}
    {#if transferReversible}
      <button
        type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken || pending}
        onclick={() => { reversalKind = 'transfer'; reason = ''; actionError = undefined; modal = 'reason'; }}
      >
        {m.transactions_investment_reverse_transfer_action()}
      </button>
    {/if}
    {#if transferReplaceable}
      <button type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken}
        onclick={() => { replacementKind = 'transfer'; }}>
        {m.transactions_investment_replace_transfer_action()}
      </button>
    {/if}
    {#if basisCorrectable}
      <button type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken}
        onclick={() => { replacementKind = 'correct_basis'; }}>
        {m.transactions_investment_correct_basis_action()}
      </button>
      <button type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken || pending}
        onclick={() => { reversalKind = 'basis_resolution'; reason = ''; actionError = undefined; modal = 'reason'; }}>
        {m.transactions_investment_withdraw_basis_action()}
      </button>
    {/if}
    {#if basisResolvable}
      <button type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken}
        onclick={() => { replacementKind = 'resolve_basis'; }}>
        {m.transactions_investment_resolve_basis_action()}
      </button>
    {/if}
    {#if chainQuery.data.can_reverse_sale && chainQuery.data.effective_transaction_id === transactionID}
      <button
        type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken || pending}
        onclick={() => { reversalKind = 'sale'; reason = ''; actionError = undefined; modal = 'reason'; }}
      >
        {m.transactions_investment_reverse_action()}
      </button>
    {/if}
    {#if chainQuery.data.can_reverse_buy && chainQuery.data.effective_transaction_id === transactionID}
      <button
        type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken || pending}
        onclick={() => { reversalKind = 'buy'; reason = ''; actionError = undefined; modal = 'reason'; }}
      >
        {m.transactions_investment_reverse_buy_action()}
      </button>
    {/if}
    {#if chainQuery.data.effective_transaction_id === transactionID &&
      chainQuery.data.operations.some((node) => node.transaction_id === transactionID && node.operation_kind === 'buy' && node.effective)}
      <button type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken}
        onclick={() => { replacementKind = 'buy'; }}>
        {m.transactions_investment_replace_buy_action()}
      </button>
    {/if}
    {#if chainQuery.data.effective_transaction_id === transactionID &&
      chainQuery.data.operations.some((node) => node.transaction_id === transactionID && node.operation_kind === 'sell' && node.effective)}
      <button type="button"
        class="inline-flex min-h-10 items-center rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover disabled:opacity-60"
        disabled={!csrfToken}
        onclick={() => { replacementKind = 'sell'; }}>
        {m.transactions_investment_replace_sale_action()}
      </button>
    {/if}
  {:else}
    <p class="text-sm text-muted">{m.transactions_investment_history_empty()}</p>
  {/if}
</section>

{#if (replacementKind === 'cash_in_lieu' || replacementKind === 'cash_in_lieu_entry') && csrfToken}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-3 py-4 backdrop-blur-sm" role="presentation">
    <div class="max-h-full w-full max-w-2xl overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface p-4 shadow-[var(--shadow-panel)] sm:p-6"
      role="dialog" aria-modal="true" aria-label={replacementKind === 'cash_in_lieu' ? m.investments_cash_in_lieu_correct() : m.investments_cash_in_lieu_title()}>
      {#if replacementKind === 'cash_in_lieu_entry' && chainQuery.data?.effective_split}
        <CashInLieuForm {csrfToken} split={{ transactionID, holdingAccountID: chainQuery.data.effective_split.holding_account_id, commodityID: chainQuery.data.effective_split.commodity_id, effectiveOn: chainQuery.data.effective_split.effective_on }} onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
      {:else if replacementQuery.isPending}<p role="status" class="text-sm text-muted">{m.investments_loading()}</p>
      {:else if replacementQuery.isError}<APIFormError error={replacementQuery.error} />
      {:else if replacementQuery.data?.operation_kind === 'cash_in_lieu' && !replacementQuery.data.already_corrected}
        <CashInLieuForm {csrfToken} correction={replacementQuery.data} onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
      {:else}<p class="text-sm text-muted">{m.transactions_investment_replace_unavailable()}</p>{/if}
      {#if replacementQuery.isError || (replacementQuery.data?.already_corrected && replacementKind === 'cash_in_lieu')}
        <button type="button" onclick={() => (replacementKind = null)} class="mt-3 rounded-[var(--radius-control)] border border-border bg-control px-3 py-2 text-sm text-foreground">{m.investments_form_cancel()}</button>
      {/if}
    </div>
  </div>
{:else if replacementKind === 'capital_return' && csrfToken && chainQuery.data?.effective_return_of_capital}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-3 py-4 backdrop-blur-sm" role="presentation">
    <div class="max-h-full w-full max-w-2xl overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface p-4 shadow-[var(--shadow-panel)] sm:p-6"
      role="dialog" aria-modal="true" aria-label={m.investments_capital_return_correct()}>
      <CapitalReturnForm {csrfToken} correction={{ transactionID, terms: chainQuery.data.effective_return_of_capital }}
        onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
    </div>
  </div>
{:else if (replacementKind === 'dividend' || replacementKind === 'reinvestment') && csrfToken}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-3 py-4 backdrop-blur-sm"
    role="presentation">
    <div class="max-h-full w-full max-w-2xl overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface p-4 shadow-[var(--shadow-panel)] sm:p-6"
      role="dialog" aria-modal="true" aria-label={replacementKind === 'dividend'
        ? m.transactions_investment_replace_dividend_title() : m.transactions_investment_replace_reinvestment_title()}>
      {#if replacementKind === 'dividend' && dividendCorrectable && chainQuery.data?.effective_dividend}
        <DividendCorrectionForm {csrfToken} {transactionID} dividend={chainQuery.data.effective_dividend}
          onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
      {:else if replacementKind === 'reinvestment' && reinvestmentCorrectable && chainQuery.data?.effective_reinvestment}
        <DividendCorrectionForm {csrfToken} {transactionID} reinvestment={chainQuery.data.effective_reinvestment}
          onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
      {:else}
        <p class="text-sm text-muted">{m.transactions_investment_replace_unavailable()}</p>
        <button type="button" class="mt-4 min-h-10 rounded-[var(--radius-control)] border border-border bg-control px-4 text-sm font-semibold text-foreground"
          onclick={() => (replacementKind = null)}>{m.investments_form_cancel()}</button>
      {/if}
    </div>
  </div>
{:else if replacementKind === 'transfer' && csrfToken && chainQuery.data?.effective_transfer}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-3 py-4 backdrop-blur-sm"
    role="presentation">
    <div class="max-h-full w-full max-w-2xl overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface p-4 shadow-[var(--shadow-panel)] sm:p-6"
      role="dialog" aria-modal="true" aria-label={chainQuery.data.effective_transfer.transfer_kind === 'external_out'
        ? m.transactions_investment_replace_transfer_out_title() : m.transactions_investment_replace_transfer_title()}>
      {#if chainQuery.data.effective_transfer.transfer_kind === 'external_in'}
        <TransferInCorrectionForm {csrfToken} {transactionID} transfer={chainQuery.data.effective_transfer}
          onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
      {:else}
        <TransferCorrectionForm {csrfToken} {transactionID} transfer={chainQuery.data.effective_transfer}
          onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
      {/if}
    </div>
  </div>
{:else if replacementKind === 'resolve_basis' && csrfToken && chainQuery.data?.effective_transfer}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-3 py-4 backdrop-blur-sm"
    role="presentation">
    <div class="max-h-full w-full max-w-2xl overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface p-4 shadow-[var(--shadow-panel)] sm:p-6"
      role="dialog" aria-modal="true" aria-label={m.investments_basis_resolution_title()}>
      <BasisResolutionForm {csrfToken} {transactionID} costCommodityID={chainQuery.data.effective_transfer.cost_commodity_id}
        onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
    </div>
  </div>
{:else if replacementKind === 'correct_basis' && csrfToken && chainQuery.data?.effective_basis_resolution}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-3 py-4 backdrop-blur-sm"
    role="presentation">
    <div class="max-h-full w-full max-w-2xl overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface p-4 shadow-[var(--shadow-panel)] sm:p-6"
      role="dialog" aria-modal="true" aria-label={m.investments_basis_resolution_correct_title()}>
      <BasisResolutionForm {csrfToken} {transactionID}
        costCommodityID={chainQuery.data.effective_basis_resolution.cost_commodity_id}
        current={chainQuery.data.effective_basis_resolution}
        onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
    </div>
  </div>
{:else if (replacementKind === 'short_sale' || replacementKind === 'short_cover') && csrfToken}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-3 py-4 backdrop-blur-sm"
    role="presentation">
    <div class="max-h-full w-full max-w-2xl overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface p-4 shadow-[var(--shadow-panel)] sm:p-6"
      role="dialog" aria-modal="true" aria-label={replacementKind === 'short_sale'
        ? m.investments_short_sale_correct_title() : m.investments_short_cover_correct_title()}>
      {#if replacementQuery.isPending}
        <p class="text-sm text-muted" role="status">{m.transactions_investment_replace_loading()}</p>
      {:else if replacementQuery.isError}
        <APIFormError error={replacementQuery.error} />
        <button type="button" class="mt-2 text-sm font-semibold text-accent" onclick={() => replacementQuery.refetch()}>{m.transactions_retry()}</button>
      {:else if replacementQuery.data?.operation_kind === replacementKind && !replacementQuery.data.already_corrected}
        <ShortTradeForm mode={replacementKind === 'short_sale' ? 'open' : 'cover'} {csrfToken} correction={replacementQuery.data}
          onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
      {:else}
        <p class="text-sm text-muted">{m.transactions_investment_replace_unavailable()}</p>
        <button type="button" class="mt-4 min-h-10 rounded-[var(--radius-control)] border border-border bg-control px-4 text-sm font-semibold text-foreground"
          onclick={() => (replacementKind = null)}>{m.investments_form_cancel()}</button>
      {/if}
    </div>
  </div>
{:else if replacementKind === 'write_off' && csrfToken}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-3 py-4 backdrop-blur-sm"
    role="presentation">
    <div class="max-h-full w-full max-w-2xl overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface p-4 shadow-[var(--shadow-panel)] sm:p-6"
      role="dialog" aria-modal="true" aria-label={m.transactions_investment_replace_write_off_title()}>
      {#if replacementQuery.isPending}
        <p class="text-sm text-muted" role="status">{m.transactions_investment_replace_loading()}</p>
      {:else if replacementQuery.isError}
        <APIFormError error={replacementQuery.error} />
        <button type="button" class="mt-2 text-sm font-semibold text-accent" onclick={() => replacementQuery.refetch()}>{m.transactions_retry()}</button>
      {:else if replacementQuery.data?.operation_kind === 'write_off' && replacementQuery.data.can_replace_sale && !replacementQuery.data.already_corrected}
        <WriteOffCorrectionForm {csrfToken} correction={replacementQuery.data} onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
      {:else}
        <p class="text-sm text-muted">{m.transactions_investment_replace_unavailable()}</p>
        <button type="button" class="mt-4 min-h-10 rounded-[var(--radius-control)] border border-border bg-control px-4 text-sm font-semibold text-foreground"
          onclick={() => (replacementKind = null)}>{m.investments_form_cancel()}</button>
      {/if}
    </div>
  </div>
{:else if replacementKind === 'split' && csrfToken && chainQuery.data?.effective_split}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-3 py-4 backdrop-blur-sm"
    role="presentation">
    <div class="max-h-full w-full max-w-2xl overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface p-4 shadow-[var(--shadow-panel)] sm:p-6"
      role="dialog" aria-modal="true" aria-label={m.transactions_investment_replace_split_title()}>
      <SplitForm {csrfToken} correction={{ transactionID, terms: chainQuery.data.effective_split }}
        onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
    </div>
  </div>
{:else if replacementKind === 'buy' || replacementKind === 'sell'}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-3 py-4 backdrop-blur-sm"
    role="presentation">
    <div class="max-h-full w-full max-w-2xl overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface p-4 shadow-[var(--shadow-panel)] sm:p-6"
      role="dialog" aria-modal="true" aria-label={replacementKind === 'buy'
        ? m.transactions_investment_replace_buy_title() : m.transactions_investment_replace_sale_title()}>
      {#if replacementQuery.isPending}
        <p class="text-sm text-muted" role="status">{m.transactions_investment_replace_loading()}</p>
      {:else if replacementQuery.isError}
        <APIFormError error={replacementQuery.error} />
        <button type="button" class="mt-2 text-sm font-semibold text-accent" onclick={() => replacementQuery.refetch()}>{m.transactions_retry()}</button>
      {:else if replacementQuery.data?.operation_kind === 'buy' && replacementKind === 'buy' && (!replacementQuery.data.imported || replacementQuery.data.source_identity_id > 0 || sourceLinkedEffectiveBuy) && !replacementQuery.data.already_corrected && csrfToken}
        <BuyForm {csrfToken} correction={replacementQuery.data} onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
      {:else if replacementQuery.data?.operation_kind === 'sell' && replacementKind === 'sell' && replacementQuery.data.can_replace_sale && !replacementQuery.data.already_corrected && csrfToken}
        <SellForm {csrfToken} correction={replacementQuery.data} onSaved={replacementSaved} onCancel={() => (replacementKind = null)} />
      {:else}
        <p class="text-sm text-muted">{m.transactions_investment_replace_unavailable()}</p>
      {/if}
      {#if replacementQuery.isPending || replacementQuery.isError || replacementQuery.data?.operation_kind !== replacementKind || (replacementQuery.data.imported && replacementQuery.data.source_identity_id === 0 && !(replacementKind === 'buy' && sourceLinkedEffectiveBuy)) || replacementQuery.data.already_corrected || (replacementKind === 'sell' && !replacementQuery.data.can_replace_sale) || !csrfToken}
        <button type="button" class="mt-4 min-h-10 rounded-[var(--radius-control)] border border-border bg-control px-4 text-sm font-semibold text-foreground"
          onclick={() => (replacementKind = null)}>{m.investments_form_cancel()}</button>
      {/if}
    </div>
  </div>
{/if}

{#if modal !== 'closed'}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/70 px-4 py-6 backdrop-blur-sm">
    <div class="max-h-full w-full max-w-lg overflow-y-auto rounded-[var(--radius-panel)] border border-border bg-surface shadow-[var(--shadow-panel)]"
      role="alertdialog" aria-modal="true" aria-labelledby="investment-reversal-title">
      <div class="border-b border-border px-4 py-3">
        <h3 id="investment-reversal-title" class="text-sm font-semibold text-foreground">
          {modal === 'reason'
            ? reversalKind === 'buy' ? m.transactions_investment_reverse_buy_title()
              : reversalKind === 'split' ? m.transactions_investment_reverse_split_title()
              : reversalKind === 'dividend' ? m.transactions_investment_reverse_dividend_title()
              : reversalKind === 'reinvestment' ? m.transactions_investment_reverse_reinvestment_title()
              : reversalKind === 'write_off' ? m.transactions_investment_reverse_write_off_title()
              : reversalKind === 'transfer' ? m.transactions_investment_reverse_transfer_title()
              : reversalKind === 'capital_return' ? m.transactions_investment_reverse_capital_return_title()
              : reversalKind === 'cash_in_lieu' ? m.investments_cash_in_lieu_reverse_title()
              : reversalKind === 'short_sale' ? m.investments_short_sale_reverse_title()
              : reversalKind === 'short_cover' ? m.investments_short_cover_reverse_title()
              : reversalKind === 'basis_resolution' ? m.transactions_investment_withdraw_basis_title()
              : m.transactions_investment_reverse_title()
            : impacts.length > 0 ? m.transactions_reconciliation_warning_title() : m.investments_gain_impact_title()}
        </h3>
        {#if modal === 'reason' || impacts.length > 0}
          <p class="mt-1 text-xs leading-5 text-muted">
            {modal === 'reason'
              ? reversalKind === 'buy' ? m.transactions_investment_reverse_buy_copy()
                : reversalKind === 'split' ? m.transactions_investment_reverse_split_copy()
                : reversalKind === 'dividend' ? m.transactions_investment_reverse_dividend_copy()
                : reversalKind === 'reinvestment' ? m.transactions_investment_reverse_reinvestment_copy()
                : reversalKind === 'write_off' ? m.transactions_investment_reverse_write_off_copy()
                : reversalKind === 'transfer' ? m.transactions_investment_reverse_transfer_copy()
                : reversalKind === 'capital_return' ? m.transactions_investment_reverse_capital_return_copy()
                : reversalKind === 'cash_in_lieu' ? m.investments_cash_in_lieu_reverse_copy()
                : reversalKind === 'short_sale' ? m.investments_short_sale_reverse_copy()
                : reversalKind === 'short_cover' ? m.investments_short_cover_reverse_copy()
                : reversalKind === 'basis_resolution' ? m.transactions_investment_withdraw_basis_copy()
                : m.transactions_investment_reverse_copy()
              : m.transactions_reconciliation_warning_copy()}
          </p>
        {/if}
      </div>

      {#if modal === 'reason'}
        <div class="px-4 py-3">
          <label for="investment-reversal-reason" class="block text-xs font-semibold uppercase tracking-[0.12em] text-muted">
            {m.transactions_investment_reverse_reason()}
          </label>
          <input id="investment-reversal-reason" type="text" bind:this={reasonInputElement} bind:value={reason} maxlength="500" required
            class="mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground outline-none focus:border-accent" />
        </div>
      {:else}
        {#if impacts.length > 0}
        <ul class="divide-y divide-border px-4 py-3 text-sm">
          {#each impacts as checkpoint (checkpoint.checkpoint_id)}
            <li class="flex gap-2 py-2 text-foreground">
              <AlertTriangle size={16} class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
              {m.transactions_reconciliation_checkpoint_label({
                account: checkpoint.account_label,
                commodity: checkpoint.commodity_code,
                date: checkpoint.statement_date
              })}
            </li>
          {/each}
        </ul>
        {/if}
        {#if gainRows.length > 0}
          <div class:border-t={impacts.length > 0} class="border-border">
            <GainImpactList rows={gainRows} refreshed={gainRefreshed} showHeading={impacts.length > 0}
              labelledBy="investment-reversal-title" />
          </div>
        {/if}
      {/if}

      <div class="px-4 pb-2"><APIFormError error={actionError} /></div>
      <div class="flex flex-wrap justify-end gap-2 border-t border-border px-4 py-3">
        <button type="button" class="min-h-10 rounded-[var(--radius-control)] border border-border bg-control px-4 text-sm font-semibold text-foreground"
          disabled={pending} onclick={closeModal}>{m.transactions_reconciliation_cancel()}</button>
        <button type="button" bind:this={confirmButtonElement} class="min-h-10 rounded-[var(--radius-control)] bg-warning px-4 text-sm font-semibold text-warning-foreground disabled:opacity-60"
          disabled={pending || (modal === 'reason' && !reason.trim())}
          onclick={modal === 'reason' ? submitReason : confirmReconciliation}>
          {modal === 'reason'
            ? m.transactions_investment_reverse_confirm()
            : impacts.length > 0 && gainRows.length > 0
              ? m.investments_gain_impact_confirm_with_reconciliation()
              : impacts.length > 0 ? m.transactions_reconciliation_confirm() : m.investments_gain_impact_confirm()}
        </button>
      </div>
    </div>
  </div>
{/if}
