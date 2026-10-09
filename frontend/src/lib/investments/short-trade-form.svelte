<script lang="ts">
  // Named short sale and cover (#173/#174). An opening receives cash like a
  // sale but opens a short lot of borrowed units; a cover pays like a buy and
  // closes short lots only. Neither is an ordinary buy or sale, so the form
  // says which side it records and never offers to cross zero.
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { parseTradeAmounts, type AmountFieldError } from '#lib/investments/form-amounts.ts';
  import TradeEconomicsFields from '#lib/investments/trade-economics-fields.svelte';
  import { correctionTradeDraft, exactTradeFields, type TradeChargeDraft } from '#lib/investments/trade-economics.ts';
  import { accountsQueryOptions, type AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryKey,
    searchInvestmentInstruments,
    recordShortSale,
    recordShortCover,
    previewShortCover,
    shortSaleReconciliationImpact,
    shortCoverReconciliationImpact,
    previewShortSaleReplacement,
    previewShortCoverReplacement,
    replaceShortSale,
    replaceShortCover,
    type InvestmentTradeCorrectionContextResponse,
    type InvestmentInstrumentResponse,
    type InvestmentTradeRequest,
    type ReconciliationImpactResponse,
    type SellPreviewResponse,
    type CostBasisMethod,
    type GainImpact
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import {
    gainAcknowledgement,
    gainImpactCurrency,
    gainImpactRows,
    hasGainChanges,
    impactNeedsReview,
    isGainAcknowledgementRefusal
  } from '#lib/investments/gain-impact.ts';
  import { formatScaledValue, costBasisMethodLabel } from '#lib/investments/investment-labels.ts';
  import { coefficientSign, negateCoefficient } from '#lib/money/amount.ts';
  import { getLocale } from '#lib/paraglide/runtime.js';

  let {
    mode,
    csrfToken,
    onSaved,
    onCancel,
    correction
  }: {
    mode: 'open' | 'cover';
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
    // A native correction (#175) pre-fills the recorded short sale or cover
    // and replaces it; the position replays behind it.
    correction?: InvestmentTradeCorrectionContextResponse;
  } = $props();

  const initialCorrection = untrack(() => correction);
  const correctionDraft = initialCorrection ? correctionTradeDraft(initialCorrection) : null;

  const cover = $derived(mode === 'cover');
  // An opening receives like a sale; a cover pays like a buy.
  const economicsSide = $derived<'buy' | 'sell'>(mode === 'cover' ? 'buy' : 'sell');
  const idPrefix = $derived(`short-${mode}`);

  const locale = getLocale();
  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let instrumentSearch = $state(initialCorrection?.commodity_code ?? '');
  let instrumentSearchDebounced = $state('');
  let instrumentDebounceTimer: ReturnType<typeof setTimeout> | undefined;
  let instrumentDropdownOpen = $state(false);
  let selectedInstrument = $state<InvestmentInstrumentResponse | null>(null);

  const instrumentSearchQuery = createQuery(() => ({
    queryKey: [...investmentInstrumentsQueryKey, 'search', instrumentSearchDebounced] as const,
    queryFn: () => searchInvestmentInstruments(instrumentSearchDebounced),
    enabled: instrumentSearchDebounced.length > 0,
    staleTime: 10_000
  }));

  function onInstrumentInput(e: Event) {
    const val = (e.target as HTMLInputElement).value;
    instrumentSearch = val;
    selectedInstrument = null;
    clearTimeout(instrumentDebounceTimer);
    instrumentDebounceTimer = setTimeout(() => {
      instrumentSearchDebounced = val;
    }, 250);
    instrumentDropdownOpen = val.length > 0;
  }

  function selectInstrument(inst: InvestmentInstrumentResponse) {
    selectedInstrument = inst;
    instrumentSearch = inst.display_name;
    instrumentDropdownOpen = false;
  }

  // The recorded instrument stays selected until the user picks another.
  const commodityID = $derived(selectedInstrument?.commodity_id ??
    (initialCorrection && instrumentSearch === initialCorrection.commodity_code ? initialCorrection.commodity_id : undefined));
  let transactionDate = $state(initialCorrection?.event_date ?? new Date().toISOString().slice(0, 10));
  let holdingAccountID = $state(String(initialCorrection?.holding_account_id ?? ''));
  let cashAccountID = $state(String(initialCorrection?.cash_account_id ?? ''));
  let quantityStr = $state(correctionDraft?.quantity ?? '');
  let cashAmountStr = $state(correctionDraft?.net ?? '');
  let exactMode = $state(correctionDraft?.exactMode ?? false);
  let grossAmountStr = $state(correctionDraft?.gross ?? '');
  let settlementDate = $state(correctionDraft?.settlementDate ?? '');
  let charges = $state<TradeChargeDraft[]>(correctionDraft?.charges ?? []);
  let costBasisMethod = $state<CostBasisMethod>(
    initialCorrection?.cost_basis_method === 'lifo' || initialCorrection?.cost_basis_method === 'average_cost'
      ? initialCorrection.cost_basis_method : 'fifo');
  let memo = $state(initialCorrection?.memo ?? '');
  let reason = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);

  let reconciliationModal = $state<{
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    gainRefreshed: boolean;
    payload: InvestmentTradeRequest;
  } | null>(null);

  let preview = $state<SellPreviewResponse | null>(null);
  let previewPending = $state(false);
  let previewError = $state<unknown>(undefined);
  let previewDebounceTimer: ReturnType<typeof setTimeout> | undefined;

  const holdingAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) =>
        (a.account_kind === 'security_holding' || a.account_kind === 'fund_holding') && a.status === 'active'
    )
  );
  const cashAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) =>
        a.account_class === 'asset' && a.status === 'active' && a.allows_postings &&
        a.account_kind !== 'security_holding' && a.account_kind !== 'fund_holding'
    )
  );
  const selectedCashAccount = $derived(cashAccounts.find((a: AccountResponse) => String(a.id) === cashAccountID));
  const currenciesByID = $derived(
    new Map<number, CurrencyResponse>((currenciesQuery.data?.currencies ?? []).map((c: CurrencyResponse) => [c.id, c]))
  );
  const modalGainRows = $derived(
    reconciliationModal?.gainImpact
      ? gainImpactRows(reconciliationModal.gainImpact.changes, gainImpactCurrency(currenciesByID), locale)
      : []
  );
  const cashCommodityID = $derived(selectedCashAccount?.default_commodity_id);
  const cashCurrencyCode = $derived(cashCommodityID ? (currenciesByID.get(cashCommodityID)?.code ?? '') : '');

  function amountErrorMessage(reason: AmountFieldError): string {
    switch (reason) {
      case 'negative':
        return m.investments_form_negative_number();
      case 'too_large':
        return m.investments_form_amount_too_large();
      case 'invalid':
        return m.investments_form_invalid_number();
    }
  }

  const fieldsReady = $derived(
    !!commodityID && holdingAccountID !== '' && cashAccountID !== '' && !!cashCommodityID &&
    quantityStr.trim() !== '' && cashAmountStr.trim() !== ''
  );
  // A cover shows its result before it is confirmed; an opening has none. A
  // correction reviews its replayed results through the replace preview.
  const canSubmit = $derived(fieldsReady && (correction ? reason.trim() !== '' :
    (!cover || (preview !== null && !previewPending))));

  type BuiltPayload = { ok: true; payload: InvestmentTradeRequest } | { ok: false; reason: AmountFieldError };

  function buildPayload(): BuiltPayload | null {
    if (!commodityID || !cashCommodityID) return null;
    const amounts = parseTradeAmounts({ quantityStr, cashAmountStr });
    if (!amounts.ok) return amounts;
    const { quantity, cashAmount } = amounts.values;
    const economics = exactMode ? exactTradeFields({ side: economicsSide, gross: grossAmountStr,
      settlementDate, charges, cashCommodityID, netValue: cashAmount.value, netScale: cashAmount.scale }) : null;
    if (economics && !economics.ok) return economics;
    return { ok: true, payload: {
      transaction_date: transactionDate,
      commodity_id: commodityID,
      holding_account_id: Number(holdingAccountID),
      cash_account_id: Number(cashAccountID),
      quantity_value: quantity.value,
      quantity_scale: quantity.scale,
      cash_amount_value: cashAmount.value,
      cash_amount_scale: cashAmount.scale,
      cash_commodity_id: cashCommodityID,
      ...(economics?.ok ? economics.fields : {}),
      ...(cover ? { cost_basis_method: costBasisMethod } : {}),
      memo: memo.trim() || undefined,
      payee_id: correction?.payee_id
    } };
  }

  $effect(() => {
    if (!cover || correction) return;
    // Track every input the preview depends on.
    void [selectedInstrument?.commodity_id, holdingAccountID, cashAccountID, quantityStr, cashAmountStr,
      costBasisMethod, cashCommodityID, transactionDate, exactMode, grossAmountStr, settlementDate, JSON.stringify(charges)];
    preview = null;
    previewError = undefined;
    clearTimeout(previewDebounceTimer);
    if (!fieldsReady) return;
    previewDebounceTimer = setTimeout(() => void triggerPreview(), 400);
  });

  async function triggerPreview() {
    const built = buildPayload();
    if (!built) return;
    if (!built.ok) {
      // A half-typed value stays quiet; an unrepresentable one will not
      // become valid by typing more.
      if (built.reason === 'too_large') previewError = new Error(amountErrorMessage(built.reason));
      return;
    }
    previewPending = true;
    previewError = undefined;
    try {
      preview = await previewShortCover(built.payload);
    } catch (err) {
      previewError = err;
    } finally {
      previewPending = false;
    }
  }

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!canSubmit) return;
    const built = buildPayload();
    if (!built) return;
    if (!built.ok) {
      formError = new Error(amountErrorMessage(built.reason));
      return;
    }
    if (correction && charges.some((charge) => !charge.treatment)) {
      formError = new Error(m.transactions_investment_replace_charge_treatment());
      return;
    }
    pending = true;
    formError = undefined;
    try {
      if (await reviewImpact(built.payload, false)) return;
      await submit(built.payload, false, '');
    } catch (err) {
      await recoverFromRefusal(err, built.payload);
    } finally {
      pending = false;
    }
  }

  function replacementRequest(payload: InvestmentTradeRequest) {
    return { reason: reason.trim(), replacement: { ...payload,
      charges: payload.charges?.map((charge) => ({ ...charge, treatment: charge.treatment! })) } };
  }

  async function reviewImpact(payload: InvestmentTradeRequest, refreshed: boolean): Promise<boolean> {
    const impact = correction
      ? cover
        ? await previewShortCoverReplacement(correction.transaction_id, { ...replacementRequest(payload),
          replacement: { ...replacementRequest(payload).replacement, cost_basis_method: costBasisMethod } })
        : await previewShortSaleReplacement(correction.transaction_id, replacementRequest(payload))
      : cover ? await shortCoverReconciliationImpact(payload) : await shortSaleReconciliationImpact(payload);
    if (!impactNeedsReview(impact)) return false;
    const gainImpact = hasGainChanges(impact.gain_impact) ? impact.gain_impact : null;
    reconciliationModal = { impacts: impact.affected_checkpoints, gainImpact, gainRefreshed: refreshed && !!gainImpact, payload };
    return true;
  }

  async function recoverFromRefusal(err: unknown, payload: InvestmentTradeRequest) {
    try {
      if (!isGainAcknowledgementRefusal(err) || !(await reviewImpact(payload, true))) formError = err;
    } catch (previewErr) {
      formError = previewErr;
    }
  }

  async function confirmOverride() {
    if (!reconciliationModal) return;
    const { payload, impacts, gainImpact } = reconciliationModal;
    reconciliationModal = null;
    pending = true;
    formError = undefined;
    try {
      await submit(payload, impacts.length > 0, gainAcknowledgement(gainImpact));
    } catch (err) {
      await recoverFromRefusal(err, payload);
    } finally {
      pending = false;
    }
  }

  async function submit(payload: InvestmentTradeRequest, override: boolean, acknowledgement: string) {
    const flags = {
      ...(override ? { reconciliation_override: true } : {}),
      ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {})
    };
    const body = { ...payload, ...flags };
    if (correction) {
      const request = { ...replacementRequest(payload), ...flags };
      if (cover) {
        await replaceShortCover(correction.transaction_id,
          { ...request, replacement: { ...request.replacement, cost_basis_method: costBasisMethod } }, csrfToken);
      } else {
        await replaceShortSale(correction.transaction_id, request, csrfToken);
      }
    } else if (cover) {
      await recordShortCover(body, csrfToken);
    } else {
      await recordShortSale(body, csrfToken);
    }
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  const COVER_METHODS: CostBasisMethod[] = ['fifo', 'lifo', 'average_cost'];
</script>

{#if reconciliationModal}
  <ReconciliationConfirm
    impacts={reconciliationModal.impacts}
    gainRows={modalGainRows}
    gainRefreshed={reconciliationModal.gainRefreshed}
    {pending}
    onCancel={() => (reconciliationModal = null)}
    onConfirm={confirmOverride}
  />
{/if}

<form onsubmit={handleSubmit} class="space-y-4" aria-labelledby="{idPrefix}-title">
  <h2 id="{idPrefix}-title" class="text-base font-semibold text-foreground">
    {correction
      ? cover ? m.investments_short_cover_correct_title() : m.investments_short_sale_correct_title()
      : cover ? m.investments_short_cover_title() : m.investments_short_sale_title()}
  </h2>
  <p class="text-sm text-muted">{correction ? m.investments_short_correct_copy()
    : cover ? m.investments_short_cover_copy() : m.investments_short_sale_copy()}</p>
  {#if correction}
    <div>
      <label for="{idPrefix}-reason" class="mb-1 block text-sm font-medium text-foreground">{m.transactions_investment_replace_reason()}</label>
      <input id="{idPrefix}-reason" type="text" bind:value={reason} maxlength="500" required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
  {/if}

  <div>
    <label for="{idPrefix}-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_date()}</label>
    <input id="{idPrefix}-date" type="date" bind:value={transactionDate} required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <div class="relative">
    <label for="{idPrefix}-instrument" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_instrument()}</label>
    <input id="{idPrefix}-instrument" type="text" value={instrumentSearch} oninput={onInstrumentInput}
      placeholder={m.investments_form_instrument_placeholder()} autocomplete="off"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground placeholder:text-muted" />
    {#if instrumentDropdownOpen && (instrumentSearchQuery.data?.instruments ?? []).length > 0}
      <ul class="absolute z-20 mt-1 max-h-48 w-full overflow-y-auto rounded-(--radius-panel) border border-border bg-surface shadow-(--shadow-panel)"
        role="listbox" aria-label={m.investments_form_instrument_results()}>
        {#each instrumentSearchQuery.data!.instruments as inst (inst.commodity_id)}
          <li>
            <button type="button" role="option" aria-selected={selectedInstrument?.commodity_id === inst.commodity_id}
              class="w-full px-3 py-2 text-left text-sm hover:bg-surface-strong/40 focus:bg-surface-strong/40"
              onclick={() => selectInstrument(inst)}>
              <span class="font-medium">{inst.symbol ?? inst.commodity_code}</span>
              <span class="ml-2 text-muted">{inst.display_name}</span>
            </button>
          </li>
        {/each}
      </ul>
    {:else if instrumentDropdownOpen && instrumentSearchQuery.isFetching}
      <div class="absolute z-20 mt-1 w-full rounded-(--radius-panel) border border-border bg-surface px-3 py-2 text-sm text-muted shadow-(--shadow-panel)">
        {m.investments_form_searching()}
      </div>
    {/if}
  </div>

  <div>
    <label for="{idPrefix}-holding-account" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_holding_account()}</label>
    <select id="{idPrefix}-holding-account" bind:value={holdingAccountID} required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
      <option value="">{m.investments_form_select_account()}</option>
      {#each holdingAccounts as acc (acc.id)}
        <option value={String(acc.id)}>{acc.name}</option>
      {/each}
    </select>
  </div>

  <div>
    <label for="{idPrefix}-quantity" class="mb-1 block text-sm font-medium text-foreground">
      {cover ? m.investments_short_cover_quantity() : m.investments_short_sale_quantity()}
    </label>
    <input id="{idPrefix}-quantity" type="text" inputmode="decimal" bind:value={quantityStr} placeholder="0.000" required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted" />
  </div>

  <div>
    <label for="{idPrefix}-cash-account" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_cash_account()}</label>
    <select id="{idPrefix}-cash-account" bind:value={cashAccountID} required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
      <option value="">{m.investments_form_select_account()}</option>
      {#each cashAccounts as acc (acc.id)}
        <option value={String(acc.id)}>{acc.name}</option>
      {/each}
    </select>
  </div>

  <div>
    <label for="{idPrefix}-cash-amount" class="mb-1 block text-sm font-medium text-foreground">
      {cover ? m.investments_short_cover_cost() : m.investments_short_sale_proceeds()}
      {#if cashCurrencyCode}<span class="ml-1 text-xs font-normal text-muted">({cashCurrencyCode})</span>{/if}
    </label>
    <input id="{idPrefix}-cash-amount" type="text" inputmode="decimal" bind:value={cashAmountStr} placeholder="0.00" required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted" />
  </div>

  <TradeEconomicsFields side={economicsSide} cashCommodityID={cashCommodityID}
    accounts={accountsQuery.data?.accounts ?? []} currencies={currenciesQuery.data?.currencies ?? []}
    bind:exactMode bind:gross={grossAmountStr} bind:settlementDate bind:charges />

  {#if cover}
    <div>
      <label for="{idPrefix}-method" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_cost_basis_method()}</label>
      <select id="{idPrefix}-method" bind:value={costBasisMethod}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {#each COVER_METHODS as method (method)}
          <option value={method}>{costBasisMethodLabel(method)}</option>
        {/each}
      </select>
    </div>
  {/if}

  <div>
    <label for="{idPrefix}-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
    <input id="{idPrefix}-memo" type="text" bind:value={memo}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  {#if cover}
    {#if previewPending}
      <div class="rounded-(--radius-panel) border border-border bg-surface p-3 text-sm text-muted">{m.investments_sell_preview_loading()}</div>
    {:else if previewError}
      <APIFormError error={previewError} id="{idPrefix}-preview-error" />
    {:else if preview}
      <div class="space-y-2 rounded-(--radius-panel) border border-border bg-surface p-3 text-sm" data-testid="short-cover-preview">
        <p class="font-medium text-foreground">{m.investments_short_cover_preview_title()}</p>
        <div class="grid grid-cols-2 gap-x-3 gap-y-1 text-xs">
          <span class="text-muted">{m.investments_sell_preview_method()}</span>
          <span class="text-right font-medium text-foreground">{costBasisMethodLabel(preview.cost_basis_method)}</span>
          {#if preview.disposal_decision.disposed_basis_value !== null && preview.disposal_decision.disposed_basis_scale !== null}
            <span class="text-muted">{m.investments_short_opening_proceeds()}</span>
            <span class="text-right font-mono text-foreground">
              {formatScaledValue(preview.disposal_decision.disposed_basis_value, preview.disposal_decision.disposed_basis_scale, locale)}
            </span>
          {/if}
          <span class="text-muted">{m.investments_short_cover_cost()}</span>
          <span class="text-right font-mono text-foreground">
            {formatScaledValue(negateCoefficient(preview.net_settlement_value), preview.net_settlement_scale, locale)}
            {#if cashCurrencyCode}<span class="ml-1 text-muted">{cashCurrencyCode}</span>{/if}
          </span>
          <span class="text-muted">{m.investments_short_cover_result()}</span>
          {#if preview.realized_gain !== null && preview.realized_gain_scale !== null}
            <span class="text-right font-mono {coefficientSign(preview.realized_gain) >= 0 ? 'text-foreground' : 'text-destructive'}"
              data-testid="short-cover-result">
              {coefficientSign(preview.realized_gain) >= 0 ? '+' : ''}{formatScaledValue(preview.realized_gain, preview.realized_gain_scale, locale)}
              {#if cashCurrencyCode}<span class="ml-1 text-muted">{cashCurrencyCode}</span>{/if}
            </span>
          {:else}
            <span class="text-right text-muted">{m.investments_gain_unresolved()}</span>
          {/if}
        </div>
      </div>
    {/if}
  {/if}

  <APIFormError error={formError} id="{idPrefix}-form-error" />

  <div class="flex justify-end gap-3">
    <button type="button" onclick={onCancel}
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    <button type="submit" disabled={!canSubmit || pending}
      class="rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
      {pending ? m.investments_sell_pending() : correction ? m.transactions_investment_replace_submit()
        : cover ? m.investments_short_cover_submit() : m.investments_short_sale_submit()}
    </button>
  </div>
</form>
