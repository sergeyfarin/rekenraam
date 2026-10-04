<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { accountsQueryOptions, type AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import { forecastQueryKey } from '#lib/api/forecast.ts';
  import { accountRegisterQueryKey, transactionsQueryKey } from '#lib/api/transactions.ts';
  import { formatLedgerAmount } from '#lib/money/amount.ts';
  import { parseMagnitude } from '#lib/investments/form-amounts.ts';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import {
    gainAcknowledgement,
    gainImpactCurrency,
    gainImpactRows,
    hasGainChanges,
    impactNeedsReview,
    isGainAcknowledgementRefusal
  } from '#lib/investments/gain-impact.ts';
  import {
    investmentGainsQueryKey,
    investmentLotsQueryKey,
    investmentPositionsQueryKey,
    previewTransferReplacement,
    replaceTransfer,
    type GainImpact,
    type InternalTransferRequest,
    type InvestmentCorrectionTransferTerms,
    type ReconciliationImpactResponse
  } from '#lib/api/investments.ts';

  // Replaces a posted internal transfer (T-119). The source holding, security
  // and basis currency are fixed; the date, destination, the quantity taken
  // from each originally selected lot (or the pooled quantity) and the
  // destination lineage may change. The server depletes the source as of the
  // transfer date, so availability is checked there, not here.
  let {
    csrfToken,
    transactionID,
    transfer,
    onSaved,
    onCancel
  }: {
    csrfToken: string;
    transactionID: number;
    transfer: InvestmentCorrectionTransferTerms;
    onSaved: () => void;
    onCancel: () => void;
  } = $props();

  const source = untrack(() => transfer);
  const pooled = source.basis_allocation === 'average_cost_pool';
  const sourceAccountID = source.source_account_id ?? 0;
  const locale = getLocale();
  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let reason = $state('');
  let effectiveOn = $state(source.effective_on);
  let destinationAccountID = $state(String(source.destination_account_id));
  let quantities = $state<Record<string, string>>(Object.fromEntries(source.lot_allocations.map((lot) =>
    [String(lot.lot_id), formatLedgerAmount(lot.quantity_value, lot.quantity_scale)])));
  let pooledQuantity = $state(source.quantity_value !== null && source.quantity_scale !== null
    ? formatLedgerAmount(source.quantity_value, source.quantity_scale) : '');
  let lineage = $state<'pooled_lot' | 'source_lots'>(source.destination_lineage === 'source_lots' ? 'source_lots' : 'pooled_lot');
  let memo = $state(source.memo);
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let reasonInputElement: HTMLInputElement | undefined = $state();

  $effect(() => {
    reasonInputElement?.focus();
  });

  let review = $state<{
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    gainRefreshed: boolean;
    payload: InternalTransferRequest;
  } | null>(null);

  const accounts = $derived(accountsQuery.data?.accounts ?? []);
  const sourceAccount = $derived(accounts.find((account: AccountResponse) => account.id === sourceAccountID));
  const destinationAccounts = $derived(accounts.filter((account: AccountResponse) =>
    (account.account_kind === 'security_holding' || account.account_kind === 'fund_holding') &&
    account.id !== sourceAccountID &&
    (account.status === 'active' || account.id === source.destination_account_id)));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency: CurrencyResponse) => [currency.id, currency])));
  const gainRows = $derived(review?.gainImpact
    ? gainImpactRows(review.gainImpact.changes, gainImpactCurrency(currenciesByID), locale) : []);
  const canSubmit = $derived(!!reason.trim() && effectiveOn !== '' && destinationAccountID !== '' &&
    (pooled ? !!pooledQuantity.trim() : Object.values(quantities).some((value) => !!value.trim())));

  function buildPayload(): InternalTransferRequest | null {
    const base = {
      effective_on: effectiveOn, source_account_id: sourceAccountID,
      destination_account_id: Number(destinationAccountID), commodity_id: source.commodity_id,
      cost_commodity_id: source.cost_commodity_id, source_evidence: source.source_evidence,
      memo: memo.trim() || undefined
    };
    if (pooled) {
      const parsed = parseMagnitude(pooledQuantity);
      if (!parsed.ok || parsed.field.value === '0') {
        formError = new TranslatedFormError(m.investments_transfer_internal_pooled_quantity_error());
        return null;
      }
      return { ...base, quantity_value: parsed.field.value, quantity_scale: parsed.field.scale, destination_lineage: lineage };
    }
    const allocations: InternalTransferRequest['lot_allocations'] = [];
    for (const lot of source.lot_allocations) {
      const draft = quantities[String(lot.lot_id)]?.trim();
      if (!draft) continue;
      const parsed = parseMagnitude(draft);
      if (!parsed.ok || parsed.field.value === '0') {
        formError = new TranslatedFormError(m.investments_transfer_internal_quantity_error());
        return null;
      }
      allocations.push({ lot_id: lot.lot_id, quantity_value: parsed.field.value, quantity_scale: parsed.field.scale });
    }
    if (allocations.length === 0) {
      formError = new TranslatedFormError(m.investments_transfer_internal_select_lot());
      return null;
    }
    return { ...base, lot_allocations: allocations };
  }

  async function handleSubmit(event: Event) {
    event.preventDefault();
    if (!canSubmit || pending) return;
    formError = undefined;
    const payload = buildPayload();
    if (!payload) return;
    pending = true;
    try {
      if (!(await reviewImpact(payload, false))) await commit(payload, false, '');
    } catch (error) {
      await recover(error, payload);
    } finally {
      pending = false;
    }
  }

  // Preview through the actual replacement writer; checkpoints or changed
  // gains go to the confirmation.
  async function reviewImpact(payload: InternalTransferRequest, refreshed: boolean): Promise<boolean> {
    const preview = await previewTransferReplacement(transactionID, { reason: reason.trim(), replacement: payload });
    if (!impactNeedsReview(preview.impact)) return false;
    const gainImpact = hasGainChanges(preview.impact.gain_impact) ? preview.impact.gain_impact : null;
    review = { impacts: preview.impact.affected_checkpoints, gainImpact, gainRefreshed: refreshed && !!gainImpact, payload };
    return true;
  }

  async function commit(payload: InternalTransferRequest, override: boolean, acknowledgement: string) {
    await replaceTransfer(transactionID, {
      reason: reason.trim(), replacement: payload,
      ...(override ? { reconciliation_override: true } : {}),
      ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {})
    }, csrfToken);
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: investmentPositionsQueryKey }),
      queryClient.invalidateQueries({ queryKey: investmentLotsQueryKey }),
      queryClient.invalidateQueries({ queryKey: investmentGainsQueryKey }),
      queryClient.invalidateQueries({ queryKey: forecastQueryKey }),
      queryClient.invalidateQueries({ queryKey: transactionsQueryKey }),
      queryClient.invalidateQueries({ queryKey: accountRegisterQueryKey })
    ]);
    onSaved();
  }

  // The gain set changed since review: show the current set, not a dead end.
  async function recover(error: unknown, payload: InternalTransferRequest) {
    try {
      if (!isGainAcknowledgementRefusal(error) || !(await reviewImpact(payload, true))) formError = error;
    } catch (previewError) {
      formError = previewError;
    }
  }

  async function confirmReview() {
    if (!review) return;
    const { payload, impacts, gainImpact } = review;
    review = null;
    pending = true;
    formError = undefined;
    try {
      await commit(payload, impacts.length > 0, gainAcknowledgement(gainImpact));
    } catch (error) {
      await recover(error, payload);
    } finally {
      pending = false;
    }
  }
</script>

{#if review}
  <ReconciliationConfirm impacts={review.impacts} {gainRows} gainRefreshed={review.gainRefreshed}
    {pending} onCancel={() => (review = null)} onConfirm={confirmReview} />
{/if}

<form onsubmit={handleSubmit} class="space-y-4" aria-busy={pending}>
  <h2 class="text-base font-semibold text-foreground">{m.transactions_investment_replace_transfer_title()}</h2>
  <p class="text-sm text-muted">{m.transactions_investment_replace_transfer_copy()}</p>

  <div>
    <label for="transfer-correction-reason" class="mb-1 block text-sm font-medium text-foreground">
      {m.transactions_investment_replace_reason()}
    </label>
    <input id="transfer-correction-reason" type="text" bind:this={reasonInputElement} bind:value={reason}
      maxlength="500" required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <p class="text-sm text-foreground">{m.investments_transfer_internal_source()}: <strong>{sourceAccount?.name ?? `#${sourceAccountID}`}</strong></p>

  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
    <div>
      <label for="transfer-correction-date" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_transfer_effective_date()}
      </label>
      <input id="transfer-correction-date" type="date" bind:value={effectiveOn} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="transfer-correction-destination" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_transfer_internal_destination()}
      </label>
      <select id="transfer-correction-destination" bind:value={destinationAccountID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {#each destinationAccounts as account (account.id)}
          <option value={String(account.id)}>{account.name}</option>
        {/each}
      </select>
    </div>
  </div>

  {#if pooled}
    <div class="space-y-2 rounded-(--radius-control) border border-border p-3">
      <label for="transfer-correction-pooled-quantity" class="block text-sm font-medium text-foreground">
        {m.investments_transfer_internal_lot_quantity()}
      </label>
      <input id="transfer-correction-pooled-quantity" type="text" inputmode="decimal" autocomplete="off"
        bind:value={pooledQuantity} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground" />
      <fieldset class="space-y-2 pt-1">
        <legend class="text-sm font-medium text-foreground">{m.investments_transfer_internal_lineage_legend()}</legend>
        {#each [
          { value: 'pooled_lot', label: m.investments_transfer_internal_lineage_pooled(), help: m.investments_transfer_internal_lineage_pooled_help() },
          { value: 'source_lots', label: m.investments_transfer_internal_lineage_source_lots(), help: m.investments_transfer_internal_lineage_source_lots_help() }
        ] as option (option.value)}
          <label class="flex items-start gap-2 text-sm text-foreground">
            <input type="radio" name="transfer-correction-lineage" value={option.value}
              checked={lineage === option.value}
              onchange={() => { lineage = option.value as 'pooled_lot' | 'source_lots'; }}
              aria-describedby={`transfer-correction-lineage-${option.value}-help`} class="mt-1" />
            <span>
              <span class="font-medium">{option.label}</span>
              <span id={`transfer-correction-lineage-${option.value}-help`} class="block text-xs text-muted">{option.help}</span>
            </span>
          </label>
        {/each}
      </fieldset>
    </div>
  {:else}
    <fieldset class="space-y-3 rounded-(--radius-control) border border-border p-3">
      <legend class="px-1 text-sm font-medium text-foreground">{m.investments_transfer_internal_lots()}</legend>
      <p class="text-xs text-muted">{m.transactions_investment_replace_transfer_lots_help()}</p>
      {#each source.lot_allocations as lot (lot.lot_id)}
        <div class="grid gap-2 border-t border-border pt-3 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center">
          <label for={`transfer-correction-lot-${lot.lot_id}`} class="text-sm text-foreground">
            {m.transactions_investment_replace_transfer_lot({ lot: String(lot.lot_id) })}
          </label>
          <input id={`transfer-correction-lot-${lot.lot_id}`} type="text" inputmode="decimal"
            value={quantities[String(lot.lot_id)] ?? ''}
            oninput={(event) => { quantities[String(lot.lot_id)] = event.currentTarget.value; }}
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground" />
        </div>
      {/each}
    </fieldset>
  {/if}

  <div>
    <label for="transfer-correction-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
    <input id="transfer-correction-memo" type="text" bind:value={memo} maxlength="500"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <APIFormError error={formError} id="transfer-correction-error" />

  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel} disabled={pending}
      class="min-h-10 rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    <button type="submit" disabled={!canSubmit || pending}
      class="min-h-10 rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
      {m.transactions_investment_replace_submit()}
    </button>
  </div>
</form>
