<script lang="ts">
  import EntryAccountStatus from '#lib/investments/entry-account-status.svelte';
  import { useSettlementCandidates } from '#lib/investments/settlement-candidates.svelte.ts';
  import { isSettlementAccount } from '#lib/investments/settlement-accounts.ts';
  import { isInvestmentHoldingAccount } from '#lib/investments/settlement-accounts.ts';
  import { investmentEntryAccountsQueryOptions } from '#lib/api/investment-entry.ts';
  import { autofocus } from '#lib/components/autofocus.ts';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { formatEditableAmount } from '#lib/money/amount.ts';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { gainImpactCurrency, gainImpactRows } from '#lib/investments/gain-impact.ts';
  import { commitReview, type CommitReviewState } from '#lib/investments/commit-review.ts';
  import type { ReviewGuards } from '#lib/investments/preview-review.ts';
  import { parseTradeAmounts, type AmountFieldError } from '#lib/investments/form-amounts.ts';
  import TradeEconomicsFields from '#lib/investments/trade-economics-fields.svelte';
  import TradeUnitPrice from '#lib/investments/trade-unit-price.svelte';
  import { useEntryActivity, useTradePair } from '#lib/transactions/entry-context.svelte.ts';
  import { correctionTradeDraft, exactTradeFields, type TradeChargeDraft } from '#lib/investments/trade-economics.ts';
  import type { AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryKey,
    searchInvestmentInstruments,
    recordBuy,
    buyReconciliationImpact,
    previewBuyReplacementReconciliation,
    replaceManualBuy,
    type InvestmentInstrumentResponse,
    type InvestmentTradeRequest,
    type InvestmentTradeCorrectionContextResponse
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { usePrefilledInstrument } from '#lib/investments/investment-prefill.svelte.ts';
  import type { InvestmentPrefill } from '#lib/investments/investment-prefill.ts';

  const ownerToday = useOwnerToday();

  let {
    csrfToken,
    onSaved,
    onCancel,
    correction,
    prefill
  }: {
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
    correction?: InvestmentTradeCorrectionContextResponse;
    prefill?: InvestmentPrefill;
  } = $props();
  const entryPrefill = untrack(() => correction ? undefined : prefill);

  const initialCorrection = untrack(() => correction);
  // A correction pre-fills from the recorded trade, never from the screen.
  const initialPrefill = untrack(() => correction ? undefined : prefill);
  const correctionDraft = initialCorrection ? correctionTradeDraft(initialCorrection) : null;

  const queryClient = useQueryClient();

  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  // Instrument autocomplete
  let instrumentSearch = $state(untrack(() => correction?.commodity_code) ?? '');
  let instrumentSearchDebounced = $state('');
  let instrumentDebounceTimer: ReturnType<typeof setTimeout> | undefined;
  let instrumentDropdownOpen = $state(false);
  let selectedInstrument = $state.raw<InvestmentInstrumentResponse | null>(null);
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
  }
  usePrefilledInstrument(initialPrefill, selectInstrument);

  // Form fields
  let transactionDate = $state(initialCorrection?.event_date ?? entryPrefill?.transactionDate ?? ownerToday());
  let holdingAccountID = $state(String(initialCorrection?.holding_account_id ?? initialPrefill?.holdingAccountID ?? ''));
  let cashAccountID = $state(String(initialCorrection?.cash_account_id ?? ''));
  let quantityStr = $state(correctionDraft?.quantity ?? '');
  let cashAmountStr = $state(correctionDraft?.net ?? '');
  let exactMode = $state(correctionDraft?.exactMode ?? false);
  let grossAmountStr = $state(correctionDraft?.gross ?? '');
  let settlementDate = $state(correctionDraft?.settlementDate ?? '');
  let charges = $state<TradeChargeDraft[]>(correctionDraft?.charges ?? []);
  let memo = $state(initialCorrection?.memo ?? entryPrefill?.memo ?? '');
  let reason = $state('');
  let pending = $state(false);
  const accountsQuery = createQuery(() => investmentEntryAccountsQueryOptions(transactionDate || ownerToday()));
  const cashAccountsQuery = createQuery(() => investmentEntryAccountsQueryOptions(settlementDate || transactionDate || ownerToday()));
  let formError = $state<unknown>(undefined);

  // New transaction's working copy (#508): this trade's own fields, the
  // quantity and unit price shared with its pair, and the shared context.
  let unitPriceStr = $state('');
  type TradeWorkingCopy = { quantity: string; unitPrice: string; cashAmount: string; gross: string;
    exactMode: boolean; settlementDate: string; charges: TradeChargeDraft[] };
  const workingCopy = useEntryActivity<TradeWorkingCopy>('buy', {
    correction: !!initialCorrection,
    values: () => ({ quantity: quantityStr, unitPrice: unitPriceStr, cashAmount: cashAmountStr, gross: grossAmountStr,
      exactMode, settlementDate, charges }),
    date: () => transactionDate,
    memo: () => memo,
    holding: () => Number(holdingAccountID) || undefined,
    instrument: () => commodityID,
    pending: () => pending
  });
  const opening = useTradePair('buy', !!initialCorrection, workingCopy,
    () => ({ quantity: quantityStr, unitPrice: unitPriceStr }));
  if (opening) {
    quantityStr = opening.quantity;
    unitPriceStr = opening.unitPrice;
    cashAmountStr = opening.cashAmount;
    grossAmountStr = opening.gross;
    exactMode = workingCopy?.exactMode ?? false;
    settlementDate = workingCopy?.settlementDate ?? '';
    charges = workingCopy?.charges ?? [];
  }
  /** The pair's quantity or price changed elsewhere, so this trade's typed total was dropped. */
  const totalDropped = opening?.recalculate ?? false;


  // A backdated buy can land inside a reconciled period. Rather than letting
  // the server refuse it with no way forward, preview the impact and let the
  // user accept the named consequences (T-53).
  // A backdated buy or a buy correction can also revise the basis and gain of
  // earlier sales without touching a reconciled balance (T-114/T-126). The
  // same confirmation lists those changes, and the commit echoes the token
  // for exactly that set.
  type TradeSubject = { payload: InvestmentTradeRequest; reason: string };
  let reconciliationModal = $state<CommitReviewState<TradeSubject> | null>(null);

  // Holding accounts: security_holding or fund_holding kinds
  const holdingAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) =>
        isInvestmentHoldingAccount(a)
    )
  );

  // Cash accounts: asset accounts that allow postings (except holding accounts)
  const cashAccounts = $derived(
    (cashAccountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) =>
        isSettlementAccount(a)
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
      ? gainImpactRows(reconciliationModal.gainImpact.changes, gainImpactCurrency(currenciesByID), getLocale())
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

  const canSubmit = $derived(!accountsQuery.isFetching && !accountsQuery.isError && !cashAccountsQuery.isFetching && !cashAccountsQuery.isError &&
    !!commodityID &&
    holdingAccounts.some((account) => String(account.id) === holdingAccountID) &&
    cashAccounts.some((account) => String(account.id) === cashAccountID) &&
    !!cashCommodityID &&
    quantityStr.trim() !== '' &&
    cashAmountStr.trim() !== ''
  );

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!canSubmit || !commodityID || !cashCommodityID) return;
    if (correction && !reason.trim()) return;
    if (correction && charges.some((charge) => !charge.treatment)) {
      formError = new Error(m.transactions_investment_replace_charge_treatment());
      return;
    }

    // Both coefficients cross JSON as strings. The quantity allows 38 digits;
    // the cash coefficient keeps its backend int64 range.
    const amounts = parseTradeAmounts({ quantityStr, cashAmountStr });
    if (!amounts.ok) {
      formError = new Error(amountErrorMessage(amounts.reason));
      return;
    }
    const { quantity, cashAmount } = amounts.values;

    const economics = exactMode ? exactTradeFields({ side: 'buy', gross: grossAmountStr,
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
      memo: memo.trim() || undefined,
      payee_id: correction?.payee_id
    };

    pending = true;
    formError = undefined;

    const subject = { payload, reason: reason.trim() };
    try {
      if (await impactReview.open(subject, false)) return;
      await submitBuy(subject, {});
    } catch (err) {
      await impactReview.recover(err, subject);
    } finally {
      pending = false;
    }
  }

  // Preview the buy and, when it would invalidate checkpoints or change
  // committed gains, hand the decision to the user rather than proceeding.
  // The server recomputes the change set at commit; when it differs from what
  // the user reviewed, the current set is shown again.
  const impactReview = commitReview({
    protocol: 'gains',
    get review() { return reconciliationModal; },
    impact: ({ payload, reason: correctionReason }: TradeSubject) => correction
      ? previewBuyReplacementReconciliation(correction.transaction_id, { reason: correctionReason,
          replacement: { ...payload, charges: payload.charges?.map((charge) => ({ ...charge, treatment: charge.treatment! })) } })
      : buyReconciliationImpact(payload),
    write: submitBuy,
    setReview: (value) => { reconciliationModal = value; },
    setPending: (value) => { pending = value; },
    setError: (error) => { formError = error; }
  });

  async function submitBuy({ payload, reason: correctionReason }: TradeSubject, guards: ReviewGuards) {
    if (correction) {
      await replaceManualBuy(correction.transaction_id, {
        reason: correctionReason,
        replacement: { ...payload, charges: payload.charges?.map((charge) => ({ ...charge, treatment: charge.treatment! })) },
        ...guards
      }, csrfToken);
    } else {
      await recordBuy({ ...payload, ...guards }, csrfToken);
    }

    await invalidateInvestmentReads(queryClient);
    onSaved();
  }
  const chooseSettlement = useSettlementCandidates(() => cashAccountsQuery.data?.accounts,
    () => Number(holdingAccountID) || undefined, () => cashAccountID, (id) => { cashAccountID = id; }, entryPrefill, untrack(() => !!correction));
</script>

{#if reconciliationModal}
  <ReconciliationConfirm
    impacts={reconciliationModal.impacts}
    gainRows={modalGainRows}
    gainRefreshed={reconciliationModal.gainRefreshed}
    {pending}
    onCancel={impactReview.cancel}
    onConfirm={impactReview.confirm}
  />
{/if}

<form onsubmit={handleSubmit} aria-busy={pending} class="space-y-4">
  <h2 id="buy-title" class="text-base font-semibold text-foreground">
    {correction ? m.transactions_investment_replace_buy_title() : m.investments_buy_title()}
  </h2>
  <EntryAccountStatus pending={accountsQuery.isFetching || cashAccountsQuery.isFetching}
    error={accountsQuery.error ?? cashAccountsQuery.error}
    onRetry={() => queryClient.invalidateQueries({ queryKey: ['api', 'accounts', 'investment-entry'] })} />
  {#if correction}
    <p class="text-sm text-muted">{m.transactions_investment_replace_buy_copy()}</p>
  {/if}

  <!-- Date -->
  <div>
    <label for="buy-date" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_date()}
    </label>
    <input
      id="buy-date"
      type="date"
      bind:value={transactionDate}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
      required
    />
  </div>

  <!-- Instrument autocomplete -->
  <div class="relative">
    <label for="buy-instrument" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_instrument()}
    </label>
    <input
      id="buy-instrument"
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
    <label for="buy-holding-account" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_holding_account()}
    </label>
    <select
      id="buy-holding-account"
      bind:value={holdingAccountID}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
      required
    >
      <option value="">{m.investments_form_select_account()}</option>
      {#if holdingAccountID && !holdingAccounts.some((account) => String(account.id) === holdingAccountID)}
        <option value={holdingAccountID} disabled>{(accountsQuery.data?.accounts ?? []).find((account) => String(account.id) === holdingAccountID)?.name ?? holdingAccountID}</option>
      {/if}
      {#each holdingAccounts as acc (acc.id)}
        <option value={String(acc.id)}>{acc.name}</option>
      {/each}
    </select>
    {#if accountsQuery.data && holdingAccountID && !holdingAccounts.some((account) => String(account.id) === holdingAccountID)}
      <p role="alert" class="mt-1 text-sm text-danger">{m.investments_activity_holding_invalid()}</p>
    {/if}
  </div>

  {#if correction}
    <div>
      <label for="buy-correction-reason" class="mb-1 block text-sm font-medium text-foreground">
        {m.transactions_investment_replace_reason()}
      </label>
      <input id="buy-correction-reason" type="text" {@attach autofocus} bind:value={reason} maxlength="500" required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
  {/if}

  <!-- Quantity -->
  <div>
    <label for="buy-quantity" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_quantity()}
    </label>
    <input
      id="buy-quantity"
      type="text"
      inputmode="decimal"
      bind:value={quantityStr}
      placeholder={formatEditableAmount('0', 3)}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted"
      required
    />
  </div>

  <!-- Cash account -->
  <div>
    <label for="buy-cash-account" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_activity_pay_from()}
    </label>
    <select onchange={chooseSettlement}
      id="buy-cash-account"
      bind:value={cashAccountID}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
      required
    >
      <option value="">{m.investments_form_select_account()}</option>
      {#if cashAccountID && !cashAccounts.some((account) => String(account.id) === cashAccountID)}
        <option value={cashAccountID} disabled>{(cashAccountsQuery.data?.accounts ?? []).find((account) => String(account.id) === cashAccountID)?.name ?? cashAccountID}</option>
      {/if}
      {#each cashAccounts as acc (acc.id)}
        <option value={String(acc.id)}>{acc.name}</option>
      {/each}
    </select>
    <p class="mt-1 text-xs text-muted">{cashAccountID ? m.investments_activity_settlement_hint() : m.investments_activity_settlement_choice()}</p>
    {#if cashAccountsQuery.data && cashAccountID && !cashAccounts.some((account) => String(account.id) === cashAccountID)}
      <p role="alert" class="mt-1 text-sm text-danger">{m.investments_activity_settlement_invalid()}</p>
    {/if}
  </div>

  <TradeUnitPrice idPrefix="buy" side="buy" bind:unitPrice={unitPriceStr} bind:cashAmount={cashAmountStr}
    bind:gross={grossAmountStr} quantity={quantityStr} {exactMode} {charges} {cashCommodityID}
    scale={cashCommodityID ? currenciesByID.get(cashCommodityID)?.standard_scale : undefined} currencyCode={cashCurrencyCode} />

  <!-- Total cost -->
  <div>
    <label for="buy-cash-amount" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_total_cost()}
      {#if cashCurrencyCode}
        <span class="ml-1 text-xs font-normal text-muted">({cashCurrencyCode})</span>
      {/if}
    </label>
    <input
      id="buy-cash-amount"
      type="text"
      inputmode="decimal"
      bind:value={cashAmountStr}
      readonly={unitPriceStr.trim() !== ''}
      aria-describedby={totalDropped && cashAmountStr === '' && unitPriceStr.trim() === '' ? 'buy-total-dropped' : undefined}
      placeholder={formatEditableAmount('0', 2)}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted"
      required
    />
    {#if totalDropped && cashAmountStr === '' && unitPriceStr.trim() === ''}
      <p id="buy-total-dropped" class="mt-1 text-xs text-muted">{m.investments_trade_total_reenter()}</p>
    {/if}
  </div>

  <TradeEconomicsFields side="buy" cashCommodityID={cashCommodityID} grossDerived={unitPriceStr.trim() !== ''}
    accounts={accountsQuery.data?.accounts ?? []} currencies={currenciesQuery.data?.currencies ?? []}
    bind:exactMode bind:gross={grossAmountStr} bind:settlementDate bind:charges />

  <!-- Memo -->
  <div>
    <label for="buy-memo" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_memo()}
    </label>
    <input
      id="buy-memo"
      type="text"
      bind:value={memo}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
    />
  </div>

  <APIFormError error={formError} id="buy-form-error" />

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
      {pending ? m.investments_buy_pending() : correction ? m.transactions_investment_replace_submit() : m.investments_buy_submit()}
    </button>
  </div>
</form>
