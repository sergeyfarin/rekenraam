<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { parseTradeAmounts, type AmountFieldError } from '#lib/investments/form-amounts.ts';
  import TradeEconomicsFields from '#lib/investments/trade-economics-fields.svelte';
  import {
    correctionLotDrafts, correctionTradeDraft, exactTradeFields, parseCorrectionLotChoices,
    type CorrectionLotDraft, type TradeChargeDraft
  } from '#lib/investments/trade-economics.ts';
  import { accountsQueryOptions, type AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryKey,
    searchInvestmentInstruments,
    previewSell,
    recordSell,
    sellReconciliationImpact,
    previewSaleReplacementReconciliation,
    replaceManualSale,
    type InvestmentInstrumentResponse,
    type InvestmentTradeRequest,
    type ReconciliationImpactResponse,
    type SellPreviewResponse,
    type CostBasisMethod,
    type InvestmentTradeCorrectionContextResponse
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
  import type { GainImpact } from '#lib/api/investments.ts';
  import { formatScaledValue, costBasisMethodLabel } from '#lib/investments/investment-labels.ts';
  import { coefficientSign } from '#lib/money/amount.ts';
  import { getLocale } from '#lib/paraglide/runtime.js';

  let {
    csrfToken,
    onSaved,
    onCancel,
    correction
  }: {
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
    correction?: InvestmentTradeCorrectionContextResponse;
  } = $props();

  const initialCorrection = untrack(() => correction);
  const correctionDraft = initialCorrection ? correctionTradeDraft(initialCorrection) : null;

  const locale = getLocale();
  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  // Instrument autocomplete
  let instrumentSearch = $state(untrack(() => correction?.commodity_code) ?? '');
  let instrumentSearchDebounced = $state('');
  let instrumentDebounceTimer: ReturnType<typeof setTimeout> | undefined;
  let instrumentDropdownOpen = $state(false);
  let selectedInstrument = $state<InvestmentInstrumentResponse | null>(null);
  // A correction may move the trade to another instrument (T-116). Until the
  // user picks one, the recorded instrument stays selected.
  const commodityID = $derived(selectedInstrument?.commodity_id ??
    (correction && instrumentSearch === correction.commodity_code ? correction.commodity_id : undefined));

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
    clearPreview();
  }

  // Form fields
  let transactionDate = $state(initialCorrection?.event_date ?? todayISO());
  let holdingAccountID = $state(String(initialCorrection?.holding_account_id ?? ''));
  let cashAccountID = $state(String(initialCorrection?.cash_account_id ?? ''));
  let quantityStr = $state(correctionDraft?.quantity ?? '');
  let cashAmountStr = $state(correctionDraft?.net ?? '');
  let exactMode = $state(correctionDraft?.exactMode ?? false);
  let grossAmountStr = $state(correctionDraft?.gross ?? '');
  let settlementDate = $state(correctionDraft?.settlementDate ?? '');
  let charges = $state<TradeChargeDraft[]>(correctionDraft?.charges ?? []);
  let costBasisMethod = $state<CostBasisMethod>((initialCorrection?.cost_basis_method as CostBasisMethod) || 'fifo');
  let lotChoices = $state<CorrectionLotDraft[]>(initialCorrection ? correctionLotDrafts(initialCorrection) : []);
  let memo = $state(initialCorrection?.memo ?? '');
  let reason = $state('');
  let reasonInputElement = $state<HTMLInputElement | undefined>();
  let pending = $state(false);
  let formError = $state<unknown>(undefined);

  $effect(() => {
    if (correction) reasonInputElement?.focus();
  });

  // See buy-form: a backdated sell must be able to proceed deliberately rather
  // than being refused with no way forward (T-53).
  // A sale correction can also change the gain of the corrected sale and of
  // later sales that replay behind it (T-126); one confirmation covers both.
  let reconciliationModal = $state<{
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    gainRefreshed: boolean;
    payload: InvestmentTradeRequest;
    reason: string;
  } | null>(null);

  // Sell preview state
  let preview = $state<SellPreviewResponse | null>(null);
  let previewPending = $state(false);
  let previewError = $state<unknown>(undefined);
  let previewDebounceTimer: ReturnType<typeof setTimeout> | undefined;

  function todayISO(): string {
    return new Date().toISOString().slice(0, 10);
  }

  function clearPreview() {
    preview = null;
    previewError = undefined;
  }

  const holdingAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) =>
        (a.account_kind === 'security_holding' || a.account_kind === 'fund_holding') &&
        a.status === 'active'
    )
  );

  const cashAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) =>
        a.account_class === 'asset' &&
        a.status === 'active' &&
        a.allows_postings &&
        a.account_kind !== 'security_holding' &&
        a.account_kind !== 'fund_holding'
    )
  );

  const selectedCashAccount = $derived(
    cashAccounts.find((a: AccountResponse) => String(a.id) === cashAccountID)
  );

  const currenciesByID = $derived(
    new Map<number, CurrencyResponse>(
      (currenciesQuery.data?.currencies ?? []).map((c: CurrencyResponse) => [c.id, c])
    )
  );
  const modalGainRows = $derived(
    reconciliationModal?.gainImpact
      ? gainImpactRows(reconciliationModal.gainImpact.changes, gainImpactCurrency(currenciesByID), locale)
      : []
  );

  const cashCommodityID = $derived(selectedCashAccount?.default_commodity_id);
  const cashCurrencyCode = $derived(
    cashCommodityID ? (currenciesByID.get(cashCommodityID)?.code ?? '') : ''
  );

  // Amount validation lives in #lib/investments/form-amounts.ts so its
  // behaviour can be pinned by name; see that module for what changed.
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

  const previewReady = $derived(
    !!commodityID &&
    holdingAccountID !== '' &&
    cashAccountID !== '' &&
    !!cashCommodityID &&
    quantityStr.trim() !== '' &&
    cashAmountStr.trim() !== ''
  );

  const canSubmit = $derived(previewReady && (!!correction || (preview !== null && !previewPending)));

  // Trigger preview debounced when inputs change
  $effect(() => {
    if (correction) return;
    // Track all reactive deps
    const _inst = selectedInstrument?.commodity_id;
    const _hold = holdingAccountID;
    const _cash = cashAccountID;
    const _qty = quantityStr;
    const _amt = cashAmountStr;
    const _method = costBasisMethod;
    const _comm = cashCommodityID;
    const _date = transactionDate;
    const _exact = exactMode;
    const _gross = grossAmountStr;
    const _settlement = settlementDate;
    const _charges = JSON.stringify(charges);

    clearPreview();
    clearTimeout(previewDebounceTimer);

    if (!previewReady) return;

    previewDebounceTimer = setTimeout(() => {
      void triggerPreview();
    }, 400);
  });

  async function triggerPreview() {
    if (!selectedInstrument || !cashCommodityID) return;

    // Both coefficients cross JSON as strings. The quantity allows 38 digits;
    // the cash coefficient keeps its backend int64 range.
    const amounts = parseTradeAmounts({ quantityStr, cashAmountStr });
    if (!amounts.ok) {
      // The preview is debounced and fires while the user is still typing, so
      // a half-entered value must stay quiet rather than flash an error. An
      // amount too large to represent is different: it will not become valid
      // by typing more, so it is surfaced immediately.
      if (amounts.reason === 'too_large') {
        previewError = new Error(amountErrorMessage(amounts.reason));
      }
      return;
    }
    const { quantity, cashAmount } = amounts.values;

    const economics = exactMode ? exactTradeFields({ side: 'sell', gross: grossAmountStr,
      settlementDate, charges, cashCommodityID, netValue: cashAmount.value, netScale: cashAmount.scale }) : null;
    if (economics && !economics.ok) {
      if (economics.reason === 'too_large') previewError = new Error(amountErrorMessage(economics.reason));
      return;
    }

    previewPending = true;
    previewError = undefined;
    preview = null;

    try {
      preview = await previewSell({
        transaction_date: transactionDate,
        commodity_id: selectedInstrument.commodity_id,
        holding_account_id: Number(holdingAccountID),
        cash_account_id: Number(cashAccountID),
        quantity_value: quantity.value,
        quantity_scale: quantity.scale,
        cash_amount_value: cashAmount.value,
        cash_amount_scale: cashAmount.scale,
        cash_commodity_id: cashCommodityID,
        ...(economics?.ok ? economics.fields : {}),
        cost_basis_method: costBasisMethod
      });
    } catch (err) {
      previewError = err;
    } finally {
      previewPending = false;
    }
  }

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!canSubmit || !commodityID || !cashCommodityID) return;
    if (correction && !reason.trim()) return;
    if (correction && charges.some((charge) => !charge.treatment)) {
      formError = new Error(m.transactions_investment_replace_charge_treatment());
      return;
    }

    // Same wire contract as the preview above — the two must agree exactly.
    // Unlike the preview this reports every rejection: the user has committed
    // by submitting, and the previous version returned silently on an
    // unparseable value, leaving the button doing nothing with no explanation.
    const amounts = parseTradeAmounts({ quantityStr, cashAmountStr });
    if (!amounts.ok) {
      formError = new Error(amountErrorMessage(amounts.reason));
      return;
    }
    const { quantity, cashAmount } = amounts.values;

    const selectedLots = correction && costBasisMethod === 'specific_lot'
      ? parseCorrectionLotChoices(lotChoices, correction.available_lots, quantity) : null;
    if (selectedLots && !selectedLots.ok) {
      formError = new Error(selectedLots.reason === 'exceeds_available'
        ? m.transactions_investment_replace_lot_exceeds()
        : selectedLots.reason === 'mismatch'
          ? m.transactions_investment_replace_lot_mismatch()
          : m.transactions_investment_replace_lot_invalid());
      return;
    }

    const economics = exactMode ? exactTradeFields({ side: 'sell', gross: grossAmountStr,
      settlementDate, charges, cashCommodityID, netValue: cashAmount.value, netScale: cashAmount.scale }) : null;
    if (economics && !economics.ok) {
      formError = new Error(amountErrorMessage(economics.reason));
      return;
    }

    const payload: InvestmentTradeRequest = {
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
      cost_basis_method: costBasisMethod,
      lot_allocations: selectedLots?.ok ? selectedLots.allocations : undefined,
      memo: memo.trim() || undefined,
      payee_id: correction?.payee_id
    };

    pending = true;
    formError = undefined;

    const correctionReason = reason.trim();
    try {
      if (await reviewImpact(payload, correctionReason, false)) return;
      await submitSell(payload, false, correctionReason, '');
    } catch (err) {
      await recoverFromRefusal(err, payload, correctionReason);
    } finally {
      pending = false;
    }
  }

  // Hand any checkpoint or gain consequence to the user rather than deciding.
  async function reviewImpact(payload: InvestmentTradeRequest, correctionReason: string, refreshed: boolean): Promise<boolean> {
    const impact = correction
      ? await previewSaleReplacementReconciliation(correction.transaction_id, {
        reason: correctionReason,
        replacement: { ...payload, cost_basis_method: payload.cost_basis_method!,
          charges: payload.charges?.map((charge) => ({ ...charge, treatment: charge.treatment! })) }
      })
      : await sellReconciliationImpact(payload);
    if (!impactNeedsReview(impact)) return false;
    const gainImpact = hasGainChanges(impact.gain_impact) ? impact.gain_impact : null;
    reconciliationModal = { impacts: impact.affected_checkpoints, gainImpact,
      gainRefreshed: refreshed && !!gainImpact, payload, reason: correctionReason };
    return true;
  }

  // A stale or missing gain acknowledgement re-previews the current set.
  async function recoverFromRefusal(err: unknown, payload: InvestmentTradeRequest, correctionReason: string) {
    try {
      if (!isGainAcknowledgementRefusal(err) || !(await reviewImpact(payload, correctionReason, true))) formError = err;
    } catch (previewErr) {
      formError = previewErr;
    }
  }

  async function confirmOverride() {
    if (!reconciliationModal) return;
    const { payload, reason: correctionReason, impacts, gainImpact } = reconciliationModal;
    reconciliationModal = null;
    pending = true;
    formError = undefined;

    try {
      await submitSell(payload, impacts.length > 0, correctionReason, gainAcknowledgement(gainImpact));
    } catch (err) {
      await recoverFromRefusal(err, payload, correctionReason);
    } finally {
      pending = false;
    }
  }

  async function submitSell(payload: InvestmentTradeRequest, override: boolean, correctionReason: string, acknowledgement: string) {
    if (correction) {
      await replaceManualSale(correction.transaction_id, {
        reason: correctionReason, reconciliation_override: override,
        replacement: { ...payload, cost_basis_method: payload.cost_basis_method!,
          charges: payload.charges?.map((charge) => ({ ...charge, treatment: charge.treatment! })) },
        ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {})
      }, csrfToken);
    } else {
      // A sale dated behind a later one replays it (T-117); echo the accepted gains.
      await recordSell({
        ...payload,
        ...(override ? { reconciliation_override: true } : {}),
        ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {})
      }, csrfToken);
    }

    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  const COST_BASIS_METHODS: CostBasisMethod[] = initialCorrection
    ? ['fifo', 'lifo', 'average_cost', 'specific_lot'] : ['fifo', 'lifo', 'average_cost'];

  // The listed lots belong to the recorded position. A sale moved to another
  // holding or instrument cannot elect them; it uses a recorded order method.
  const positionMoved = $derived(!!correction &&
    (Number(holdingAccountID) !== correction.holding_account_id || commodityID !== correction.commodity_id));
  const availableMethods = $derived(positionMoved
    ? COST_BASIS_METHODS.filter((method) => method !== 'specific_lot') : COST_BASIS_METHODS);
  $effect(() => {
    if (positionMoved && costBasisMethod === 'specific_lot') costBasisMethod = 'fifo';
  });

  function replacementMethodLabel(method: CostBasisMethod): string {
    switch (method) {
      case 'fifo': return m.transactions_investment_replace_method_fifo();
      case 'lifo': return m.transactions_investment_replace_method_lifo();
      case 'average_cost': return m.transactions_investment_replace_method_average();
      case 'specific_lot': return m.transactions_investment_replace_method_specific();
    }
  }

  function formatGain(gain: string, scale: number): string {
    return formatScaledValue(gain, scale, locale);
  }

  function chargeKindLabel(kind: string): string {
    switch (kind) {
      case 'commission': return m.investments_trade_commission();
      case 'transaction_tax': return m.investments_trade_transaction_tax();
      case 'other_fee': return m.investments_trade_other_fee();
      case 'rebate': return m.investments_trade_rebate();
      default: return m.investments_trade_charge();
    }
  }
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

<form onsubmit={handleSubmit} class="space-y-4">
  <h2 class="text-base font-semibold text-foreground">
    {correction ? m.transactions_investment_replace_sale_title() : m.investments_sell_title()}
  </h2>
  {#if correction}
    <p class="text-sm text-muted">{m.transactions_investment_replace_sale_copy()}</p>
  {/if}

  <!-- Date -->
  <div>
    <label for="sell-date" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_date()}
    </label>
    <input
      id="sell-date"
      type="date"
      bind:value={transactionDate}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
      required
    />
  </div>

  <!-- Instrument autocomplete -->
  <div class="relative">
    <label for="sell-instrument" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_instrument()}
    </label>
    <input
      id="sell-instrument"
      type="text"
      value={instrumentSearch}
      oninput={onInstrumentInput}
      placeholder={m.investments_form_instrument_placeholder()}
      autocomplete="off"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground placeholder:text-muted"
    />
    {#if instrumentDropdownOpen && (instrumentSearchQuery.data?.instruments ?? []).length > 0}
      <ul
        class="absolute z-20 mt-1 max-h-48 w-full overflow-y-auto rounded-(--radius-panel) border border-border bg-surface shadow-(--shadow-panel)"
        role="listbox"
        aria-label={m.investments_form_instrument_results()}
      >
        {#each instrumentSearchQuery.data!.instruments as inst (inst.commodity_id)}
          <li>
            <button
              type="button"
              role="option"
              aria-selected={selectedInstrument?.commodity_id === inst.commodity_id}
              class="w-full px-3 py-2 text-left text-sm hover:bg-surface-strong/40 focus:bg-surface-strong/40"
              onclick={() => selectInstrument(inst)}
            >
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

  <!-- Holding account -->
  <div>
    <label for="sell-holding-account" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_holding_account()}
    </label>
    <select
      id="sell-holding-account"
      bind:value={holdingAccountID}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
      required
    >
      <option value="">{m.investments_form_select_account()}</option>
      {#each holdingAccounts as acc (acc.id)}
        <option value={String(acc.id)}>{acc.name}</option>
      {/each}
    </select>
  </div>

  {#if correction}
    <div>
      <label for="sell-correction-reason" class="mb-1 block text-sm font-medium text-foreground">
        {m.transactions_investment_replace_reason()}
      </label>
      <input id="sell-correction-reason" type="text" bind:this={reasonInputElement} bind:value={reason}
        maxlength="500" required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
  {/if}

  <!-- Quantity -->
  <div>
    <label for="sell-quantity" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_quantity()}
    </label>
    <input
      id="sell-quantity"
      type="text"
      inputmode="decimal"
      bind:value={quantityStr}
      placeholder="0.000"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted"
      required
    />
  </div>

  <!-- Cash account -->
  <div>
    <label for="sell-cash-account" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_cash_account()}
    </label>
    <select
      id="sell-cash-account"
      bind:value={cashAccountID}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
      required
    >
      <option value="">{m.investments_form_select_account()}</option>
      {#each cashAccounts as acc (acc.id)}
        <option value={String(acc.id)}>{acc.name}</option>
      {/each}
    </select>
  </div>

  <!-- Proceeds -->
  <div>
    <label for="sell-cash-amount" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_proceeds()}
      {#if cashCurrencyCode}
        <span class="ml-1 text-xs font-normal text-muted">({cashCurrencyCode})</span>
      {/if}
    </label>
    <input
      id="sell-cash-amount"
      type="text"
      inputmode="decimal"
      bind:value={cashAmountStr}
      placeholder="0.00"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted"
      required
    />
  </div>

  <TradeEconomicsFields side="sell" cashCommodityID={cashCommodityID}
    accounts={accountsQuery.data?.accounts ?? []} currencies={currenciesQuery.data?.currencies ?? []}
    bind:exactMode bind:gross={grossAmountStr} bind:settlementDate bind:charges />

  <!-- Cost-basis method -->
  <div>
    <label for="sell-method" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_cost_basis_method()}
    </label>
    <select
      id="sell-method"
      bind:value={costBasisMethod}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
    >
      {#each availableMethods as method (method)}
        <option value={method}>{correction ? replacementMethodLabel(method) : costBasisMethodLabel(method)}</option>
      {/each}
    </select>
    {#if positionMoved}
      <p class="mt-1 text-xs text-muted">{m.transactions_investment_replace_moved_lots_hint()}</p>
    {/if}
  </div>

  {#if correction && costBasisMethod === 'specific_lot'}
    <fieldset class="space-y-3 rounded-(--radius-control) border border-border p-3">
      <legend class="px-1 text-sm font-medium text-foreground">{m.transactions_investment_replace_lots_title()}</legend>
      <p class="text-xs text-muted">{m.transactions_investment_replace_lots_copy()}</p>
      {#each lotChoices as choice, index (index)}
        <div class="grid grid-cols-1 gap-2 rounded-(--radius-control) border border-border p-2 sm:grid-cols-[minmax(0,1fr)_8rem_auto]">
          <label class="text-xs text-muted">{m.transactions_investment_replace_lot()}
            <select bind:value={choice.lotID} required
              class="mt-1 w-full rounded-(--radius-control) border border-border bg-control px-2 py-2 text-sm text-foreground">
              <option value="">{m.transactions_investment_replace_lot_select()}</option>
              {#each correction.available_lots as lot (lot.lot_id)}
                <option value={String(lot.lot_id)}>
                  #{lot.lot_id} · {lot.opened_on} · {formatScaledValue(lot.quantity_value, lot.quantity_scale, locale)}
                </option>
              {/each}
            </select>
          </label>
          <label class="text-xs text-muted">{m.investments_form_quantity()}
            <input type="text" inputmode="decimal" bind:value={choice.quantity} required
              class="mt-1 w-full rounded-(--radius-control) border border-border bg-control px-2 py-2 text-sm font-mono text-foreground" />
          </label>
          <button type="button" onclick={() => (lotChoices = lotChoices.filter((_, current) => current !== index))}
            class="self-end rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
            {m.transactions_investment_replace_lot_remove()}
          </button>
        </div>
      {/each}
      <button type="button" onclick={() => (lotChoices = [...lotChoices, { lotID: '', quantity: '' }])}
        class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {m.transactions_investment_replace_lot_add()}
      </button>
    </fieldset>
  {/if}

  <!-- Memo -->
  <div>
    <label for="sell-memo" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_memo()}
    </label>
    <input
      id="sell-memo"
      type="text"
      bind:value={memo}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
    />
  </div>

  <!-- Sell preview panel -->
  {#if correction}
    <p class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-xs text-muted">
      {m.transactions_investment_replace_sale_gain_copy()}
    </p>
  {:else if previewPending}
    <div class="rounded-(--radius-panel) border border-border bg-surface p-3 text-sm text-muted">
      {m.investments_sell_preview_loading()}
    </div>
  {:else if previewError}
    <APIFormError error={previewError} id="sell-preview-error" />
  {:else if preview}
    <div class="rounded-(--radius-panel) border border-border bg-surface p-3 text-sm space-y-2">
      <p class="font-medium text-foreground">{m.investments_sell_preview_title()}</p>
      <div class="grid grid-cols-2 gap-x-3 gap-y-1 text-xs">
        <span class="text-muted">{m.investments_sell_preview_method()}</span>
        <span class="text-right font-medium text-foreground">{costBasisMethodLabel(preview.cost_basis_method)}</span>
        {#if preview.gross_amount_value !== undefined}
          <span class="text-muted">{m.investments_trade_gross()}</span>
          <span class="text-right font-mono text-foreground">{formatGain(preview.gross_amount_value, preview.gross_amount_scale)} {cashCurrencyCode}</span>
          {#each preview.charges as charge}
            <span class="text-muted">{chargeKindLabel(charge.kind)}</span>
            <span class="text-right font-mono text-foreground">{formatGain(charge.amount_value, charge.amount_scale)}</span>
          {/each}
        {/if}
        <span class="text-muted">{m.investments_sell_preview_proceeds()}</span>
        <span class="text-right font-mono text-foreground">
          {formatGain(preview.net_settlement_value, preview.net_settlement_scale)}
          {#if cashCurrencyCode}<span class="ml-1 text-muted">{cashCurrencyCode}</span>{/if}
        </span>
        <span class="text-muted">{m.investments_sell_preview_gain()}</span>
        <span class="text-right font-mono {coefficientSign(preview.realized_gain) >= 0 ? 'text-foreground' : 'text-destructive'}">
          {coefficientSign(preview.realized_gain) >= 0 ? '+' : ''}{formatGain(preview.realized_gain, preview.realized_gain_scale)}
          {#if cashCurrencyCode}<span class="ml-1 text-muted">{cashCurrencyCode}</span>{/if}
        </span>
      </div>
      {#if preview.allocations.length > 0}
        <details class="mt-1">
          <summary class="cursor-pointer text-xs text-muted hover:text-foreground">
            {m.investments_sell_preview_allocations({ count: preview.allocations.length })}
          </summary>
          <ul class="mt-2 space-y-1">
            {#each preview.allocations as alloc (alloc.lot_id)}
              <li class="grid grid-cols-[auto_1fr_1fr] gap-x-3 text-xs">
                <span class="text-muted">#{alloc.lot_id}</span>
                <span class="text-right font-mono text-foreground">
                  {formatScaledValue(alloc.quantity_value, alloc.quantity_scale, locale)}
                </span>
                <span class="text-right font-mono text-muted">
                  {m.investments_sell_preview_basis()} {formatScaledValue(alloc.cost_basis_value, alloc.cost_basis_scale, locale)}
                </span>
              </li>
            {/each}
          </ul>
        </details>
      {/if}
    </div>
  {/if}

  <APIFormError error={formError} id="sell-form-error" />

  <div class="flex justify-end gap-3">
    <button
      type="button"
      onclick={onCancel}
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
    >
      {m.investments_form_cancel()}
    </button>
    <button
      type="submit"
      disabled={!canSubmit || pending || (!!correction && !reason.trim())}
      class="rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
    >
      {pending ? m.investments_sell_pending() : correction ? m.transactions_investment_replace_submit() : m.investments_sell_submit()}
    </button>
  </div>
</form>
