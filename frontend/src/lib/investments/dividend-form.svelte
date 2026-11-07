<script lang="ts">
  import EntryAccountStatus from '#lib/investments/entry-account-status.svelte';
  import { useSettlementCandidates } from '#lib/investments/settlement-candidates.svelte.ts';
  import { isInvestmentHoldingAccount } from '#lib/investments/settlement-accounts.ts';
  import { investmentEntryAccountsQueryOptions } from '#lib/api/investment-entry.ts';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { untrack } from 'svelte';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import type { AccountResponse } from '#lib/api/accounts.ts';
  import { formatEditableAmount } from '#lib/money/amount.ts';
  import { parseDividendAmounts, type AmountFieldError } from '#lib/investments/form-amounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryKey,
    searchInvestmentInstruments,
    getDividendDefaults,
    recordDividend,
    recordReinvestedDividend,
    dividendReconciliationImpact,
    reinvestedDividendReconciliationImpact,
    type InvestmentInstrumentResponse,
    type DividendDefaultResponse,
    type DividendRequest,
    type ReinvestedDividendRequest,
    type ReconciliationImpactResponse
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import {
    gainAcknowledgement,
    gainImpactCurrency,
    gainImpactRows,
    hasGainChanges,
    impactNeedsReview,
    isStaleImpactRefusal,
    checkpointOverride
  } from '#lib/investments/gain-impact.ts';
  import type { GainImpact } from '#lib/api/investments.ts';

  import { usePrefilledInstrument } from '#lib/investments/investment-prefill.svelte.ts';
  import type { InvestmentPrefill } from '#lib/investments/investment-prefill.ts';
  import { isSettlementAccount } from '#lib/investments/settlement-accounts.ts';

  const ownerToday = useOwnerToday();

  let {
    mode = 'cash',
    csrfToken,
    onSaved,
    onCancel,
    prefill
  }: {
    mode?: 'cash' | 'withholding' | 'reinvested';
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
    prefill?: InvestmentPrefill;
  } = $props();
  const entryPrefill = untrack(() => prefill);
  const initialPrefill = untrack(() => prefill);

  const queryClient = useQueryClient();

  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  const dividendDefaultsQuery = createQuery(() => ({
    queryKey: ['api', 'investments', 'dividend-defaults'] as const,
    queryFn: () => getDividendDefaults(),
    staleTime: 60_000
  }));

  // Instrument autocomplete
  let instrumentSearch = $state('');
  let instrumentSearchDebounced = $state('');
  let instrumentDebounceTimer: ReturnType<typeof setTimeout> | undefined;
  let instrumentDropdownOpen = $state(false);
  let selectedInstrument = $state.raw<InvestmentInstrumentResponse | null>(null);

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
    applyDefaultsForInstrument(inst);
  }
  usePrefilledInstrument(initialPrefill, selectInstrument);

  // Form fields
  let transactionDate = $state(entryPrefill?.transactionDate ?? ownerToday());
  // cash dividend and separate withholding only
  let exDate = $state('');
  let cashAccountID = $state('');
  let reinvestmentCurrencyID = $state('');
  let incomeAccountID = $state('');
  let withholdingAccountID = $state('');
  let amountStr = $state('');
  let withholdingStr = $state('');
  let showWithholding = $state(false);
  // reinvested only
  let holdingAccountID = $state(initialPrefill?.holdingAccountID === undefined ? '' : String(initialPrefill.holdingAccountID));
  let quantityStr = $state('');
  let memo = $state(entryPrefill?.memo ?? '');
  let pending = $state(false);
  const accountsQuery = createQuery(() => investmentEntryAccountsQueryOptions(transactionDate || ownerToday()));
  let formError = $state<unknown>(undefined);

  // Both dividend shapes can be backdated into a reconciled period, so both get
  // the same named confirmation as the trade forms (T-53). The pending payload
  // is tagged so the confirm handler knows which route to retry.
  type PendingDividend =
    | { mode: 'cash'; payload: DividendRequest }
    | { mode: 'reinvest'; payload: ReinvestedDividendRequest };
  // A backdated reinvestment opens an earlier lot and can change the basis and
  // gain of later sales (T-126); the confirmation lists those beside checkpoints.
  let reconciliationModal = $state<{
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    gainRefreshed: boolean;
    pending: PendingDividend;
  } | null>(null);

  // The backend settlement role: cash, or a liability such as a margin loan.
  const cashAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter((a: AccountResponse) => isSettlementAccount(a))
  );

  const holdingAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) =>
        isInvestmentHoldingAccount(a)
    )
  );

  const incomeAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) => a.account_class === 'income' && a.status === 'active' && a.allows_postings
    )
  );

  const withholdingAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) => ['asset', 'liability', 'expense'].includes(a.account_class) &&
        !a.system_role && !['security_holding', 'fund_holding'].includes(a.account_kind) && a.status === 'active' && a.allows_postings
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

  const cashCommodityID = $derived(mode === 'reinvested' ? Number(reinvestmentCurrencyID) || undefined : selectedCashAccount?.default_commodity_id);
  const cashCurrencyCode = $derived(
    cashCommodityID ? (currenciesByID.get(cashCommodityID)?.code ?? '') : ''
  );

  function applyDefaultsForInstrument(inst: InvestmentInstrumentResponse) {
    const defaults = dividendDefaultsQuery.data?.defaults ?? [];
    // Find a default that matches this instrument's commodity or is the global default (no commodity_id)
    const match: DividendDefaultResponse | undefined =
      defaults.find((d) => d.commodity_id === inst.commodity_id && d.status === 'active') ??
      defaults.find((d) => d.commodity_id === undefined && d.status === 'active');
    if (!match) return;
    if (incomeAccountID === '') incomeAccountID = String(match.income_account_id);
    if (withholdingAccountID === '' && match.withholding_account_id) {
      withholdingAccountID = String(match.withholding_account_id);
    }
    if (withholdingStr === '' && match.default_withholding_value && match.default_withholding_scale !== undefined) {
      withholdingStr = formatEditableAmount(String(match.default_withholding_value), match.default_withholding_scale);
      showWithholding = true;
    }
  }

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

  const canSubmitCash = $derived(
    cashAccounts.some((account) => String(account.id) === cashAccountID) &&
    !!cashCommodityID &&
    (mode === 'withholding' ? withholdingAccountID !== '' : incomeAccountID !== '') &&
    amountStr.trim() !== ''
  );

  const canSubmitReinvested = $derived(
    !!selectedInstrument &&
    holdingAccounts.some((account) => String(account.id) === holdingAccountID) &&
    !!cashCommodityID &&
    quantityStr.trim() !== '' &&
    amountStr.trim() !== ''
  );

  const canSubmit = $derived(!accountsQuery.isFetching && !accountsQuery.isError && mode !== 'reinvested' ? canSubmitCash : canSubmitReinvested);

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!canSubmit || !cashCommodityID) return;

    // Every field is validated up front, before any request is built. The
    // previous version returned silently on an unparseable amount or quantity,
    // so a typo left the submit button doing nothing with no explanation.
    const amounts = parseDividendAmounts({
      amountStr,
      withholdingStr,
      includeWithholding: mode === 'cash' && showWithholding,
      quantityStr,
      includeQuantity: mode === 'reinvested'
    });
    if (!amounts.ok) {
      formError = new Error(amountErrorMessage(amounts.reason));
      return;
    }
    const { amount, withholding, quantity } = amounts.values;

    pending = true;
    formError = undefined;

    try {
      if (mode !== 'reinvested') {
        const payload: DividendRequest = {
          transaction_date: transactionDate,
          withholding_only: mode === 'withholding',
          commodity_id: selectedInstrument?.commodity_id,
          ex_date: exDate || undefined,
          cash_account_id: Number(cashAccountID),
          cash_commodity_id: cashCommodityID,
          income_account_id: mode !== 'withholding' && incomeAccountID ? Number(incomeAccountID) : undefined,
          amount_value: amount.value,
          amount_scale: amount.scale,
          withholding_value: withholding?.value,
          withholding_scale: withholding?.scale,
          withholding_account_id:
            (mode === 'withholding' || withholding) && withholdingAccountID ? Number(withholdingAccountID) : undefined,
          memo: memo.trim() || undefined
        };

        if (await reviewCash(payload)) return;
        await recordDividend(payload, csrfToken);
      } else {
        if (!selectedInstrument || !quantity) return;
        // Both coefficients cross JSON as strings. Quantity allows 38 digits;
        // amount_value keeps the backend int64 range.
        const payload: ReinvestedDividendRequest = {
          transaction_date: transactionDate,
          commodity_id: selectedInstrument.commodity_id,
          holding_account_id: Number(holdingAccountID),
          income_account_id: incomeAccountID ? Number(incomeAccountID) : undefined,
          quantity_value: quantity.value,
          quantity_scale: quantity.scale,
          amount_value: amount.value,
          amount_scale: amount.scale,
          cash_commodity_id: cashCommodityID,
          memo: memo.trim() || undefined
        };

        lastReinvestment = payload;
        if (await reviewReinvestment(payload, false)) return;
        await recordReinvestedDividend(payload, csrfToken);
      }

      await refreshAfterSave();
    } catch (err) {
      await recoverFromRefusal(err);
    } finally {
      pending = false;
    }
  }

  async function reviewCash(payload: DividendRequest): Promise<boolean> {
    const impact = await dividendReconciliationImpact(payload);
    if (impact.affected_checkpoints.length === 0) return false;
    reconciliationModal = { impacts: impact.affected_checkpoints, gainImpact: null, gainRefreshed: false, pending: { mode: 'cash', payload } };
    return true;
  }

  async function reviewReinvestment(payload: ReinvestedDividendRequest, refreshed: boolean): Promise<boolean> {
    const impact = await reinvestedDividendReconciliationImpact(payload);
    if (!impactNeedsReview(impact)) return false;
    const gainImpact = hasGainChanges(impact.gain_impact) ? impact.gain_impact : null;
    reconciliationModal = { impacts: impact.affected_checkpoints, gainImpact,
      gainRefreshed: refreshed && !!gainImpact, pending: { mode: 'reinvest', payload } };
    return true;
  }

  // The server recomputed the gain change set at commit and it differs from
  // what was accepted: show the current set instead of a dead end.
  let lastReinvestment: ReinvestedDividendRequest | null = null;
  async function recoverFromRefusal(err: unknown) {
    try {
      if (!isStaleImpactRefusal(err) || !lastReinvestment || !(await reviewReinvestment(lastReinvestment, true))) {
        formError = err;
      }
    } catch (previewErr) {
      formError = previewErr;
    }
  }

  async function confirmOverride() {
    if (!reconciliationModal) return;
    const { pending: target, impacts, gainImpact } = reconciliationModal;
    reconciliationModal = null;
    pending = true;
    formError = undefined;

    try {
      if (target.mode === 'cash') {
        try {
          await recordDividend({ ...target.payload, ...checkpointOverride(impacts) }, csrfToken);
        } catch (err) {
          // A statement finished since the review: show the current set (#304).
          if (isStaleImpactRefusal(err) && (await reviewCash(target.payload))) return;
          throw err;
        }
      } else {
        const acknowledgement = gainAcknowledgement(gainImpact);
        lastReinvestment = target.payload;
        await recordReinvestedDividend({
          ...target.payload,
          ...checkpointOverride(impacts),
          ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {})
        }, csrfToken);
      }

      await refreshAfterSave();
    } catch (err) {
      await recoverFromRefusal(err);
    } finally {
      pending = false;
    }
  }

  async function refreshAfterSave() {
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  const title = $derived(mode === 'withholding' ? m.investments_withholding_title() : mode === 'cash' ? m.investments_dividend_title() : m.investments_reinvested_title());
  const submitLabel = $derived(
    pending
      ? (mode === 'withholding' ? m.investments_withholding_pending() : mode === 'cash' ? m.investments_dividend_pending() : m.investments_reinvested_pending())
      : (mode === 'withholding' ? m.investments_record_withholding() : mode === 'cash' ? m.investments_dividend_submit() : m.investments_reinvested_submit())
  );
  const chooseSettlement = useSettlementCandidates(() => accountsQuery.data?.accounts,
    () => Number(holdingAccountID) || undefined, () => cashAccountID, (id) => { cashAccountID = id; }, entryPrefill, untrack(() => mode === 'reinvested'));
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

<form onsubmit={handleSubmit} aria-busy={pending} class="space-y-4">
  <h2 id={mode === 'withholding' ? 'withholding-title' : mode === 'cash' ? 'dividend-title' : 'reinvested-dividend-title'} class="text-base font-semibold text-foreground">{title}</h2>
  <EntryAccountStatus pending={accountsQuery.isFetching}
    error={accountsQuery.error}
    onRetry={() => queryClient.invalidateQueries({ queryKey: ['api', 'accounts', 'investment-entry'] })} />

  <!-- Date -->
  <div>
    <label for="div-date" class="mb-1 block text-sm font-medium text-foreground">
      {mode === 'reinvested' ? m.investments_form_date() : m.investments_form_payment_date()}
    </label>
    <input
      id="div-date"
      type="date"
      bind:value={transactionDate}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
      required
    />
  </div>

  {#if mode !== 'reinvested'}
    <div>
      <label for="div-ex-date" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_dividend_ex_date()}
        <span class="ml-1 text-xs font-normal text-muted">({m.investments_form_optional()})</span>
      </label>
      <input
        id="div-ex-date"
        type="date"
        bind:value={exDate}
        aria-describedby="div-ex-date-hint"
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
      />
      <p id="div-ex-date-hint" class="mt-1 text-xs text-muted">{m.investments_dividend_ex_date_hint()}</p>
    </div>
  {/if}

  <!-- Instrument autocomplete (optional for cash dividend, required for reinvested) -->
  <div class="relative">
    <label for="div-instrument" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_instrument()}
      {#if mode !== 'reinvested'}
        <span class="ml-1 text-xs font-normal text-muted">({m.investments_form_optional()})</span>
      {/if}
    </label>
    {#if mode !== 'reinvested'}
      <p class="mb-2 text-xs text-muted">{m.investments_dividend_payer_help()}</p>
    {/if}
    <input
      id="div-instrument"
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

  <!-- Holding account (reinvested only) -->
  {#if mode === 'reinvested'}
    <div>
      <label for="div-holding-account" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_form_holding_account()}
      </label>
      <select
        id="div-holding-account"
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
  {/if}

  {#if mode === 'reinvested'}
    <div>
      <label for="div-cost-currency" class="mb-1 block text-sm font-medium text-foreground">{m.investments_activity_cost_currency()}</label>
      <select id="div-cost-currency" bind:value={reinvestmentCurrencyID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_transfer_select_currency()}</option>
        {#each (currenciesQuery.data?.currencies ?? []).filter((currency) => currency.status === 'active') as currency (currency.id)}
          <option value={String(currency.id)}>{currency.code}</option>
        {/each}
      </select>
    </div>
  {:else}
  <!-- Cash account -->
  <div>
    <label for="div-cash-account" class="mb-1 block text-sm font-medium text-foreground">
      {mode === 'withholding' ? m.investments_activity_pay_from() : m.investments_activity_pay_into()}
    </label>
    <select onchange={chooseSettlement}
      id="div-cash-account"
      bind:value={cashAccountID}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
      required
    >
      <option value="">{m.investments_form_select_account()}</option>
      {#if cashAccountID && !cashAccounts.some((account) => String(account.id) === cashAccountID)}
        <option value={cashAccountID} disabled>{(accountsQuery.data?.accounts ?? []).find((account) => String(account.id) === cashAccountID)?.name ?? cashAccountID}</option>
      {/if}
      {#each cashAccounts as acc (acc.id)}
        <option value={String(acc.id)}>{acc.name}</option>
      {/each}
    </select>
    <p class="mt-1 text-xs text-muted">{cashAccountID ? m.investments_activity_settlement_hint() : m.investments_activity_settlement_choice()}</p>
    {#if accountsQuery.data && cashAccountID && !cashAccounts.some((account) => String(account.id) === cashAccountID)}
      <p role="alert" class="mt-1 text-sm text-danger">{m.investments_activity_settlement_invalid()}</p>
    {/if}
  </div>

  {/if}

  <!-- Quantity (reinvested only) -->
  {#if mode === 'reinvested'}
    <div>
      <label for="div-quantity" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_form_quantity()}
      </label>
      <input
        id="div-quantity"
        type="text"
        inputmode="decimal"
        bind:value={quantityStr}
        placeholder={formatEditableAmount('0', 3)}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted"
        required
      />
    </div>
  {/if}

  <!-- Amount -->
  <div>
    <label for="div-amount" class="mb-1 block text-sm font-medium text-foreground">
      {mode === 'withholding' ? m.investments_form_withholding_amount() : m.investments_form_dividend_amount()}
      {#if cashCurrencyCode}
        <span class="ml-1 text-xs font-normal text-muted">({cashCurrencyCode})</span>
      {/if}
    </label>
    <input
      id="div-amount"
      type="text"
      inputmode="decimal"
      bind:value={amountStr}
      placeholder={formatEditableAmount('0', 2)}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted"
      required
    />
  </div>

  {#if mode !== 'withholding'}
    <!-- Income account -->
    <div>
      <label for="div-income-account" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_form_income_account()}
        <span class="ml-1 text-xs font-normal text-muted">({m.investments_form_optional()})</span>
      </label>
      <select
        id="div-income-account"
        bind:value={incomeAccountID}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
      >
        <option value="">{m.investments_form_select_account()}</option>
        {#each incomeAccounts as acc (acc.id)}
          <option value={String(acc.id)}>{acc.name}</option>
        {/each}
      </select>
    </div>

  {:else}
    <div>
      <label for="div-withholding-account" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_withholding_account()}</label>
      <select id="div-withholding-account" bind:value={withholdingAccountID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_form_select_account()}</option>
        {#each withholdingAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
      </select>
    </div>
  {/if}
  <!-- Withholding tax (cash dividend only) -->
  {#if mode === 'cash'}
    <div>
      <label class="flex items-center gap-2 text-sm font-medium text-foreground">
        <input type="checkbox" bind:checked={showWithholding} class="rounded" />
        {m.investments_form_withholding_toggle()}
      </label>
    </div>

    {#if showWithholding}
      <div class="space-y-3 rounded-(--radius-panel) border border-border p-3">
        <div>
          <label for="div-withholding" class="mb-1 block text-sm font-medium text-foreground">
            {m.investments_form_withholding_amount()}
            {#if cashCurrencyCode}
              <span class="ml-1 text-xs font-normal text-muted">({cashCurrencyCode})</span>
            {/if}
          </label>
          <input
            id="div-withholding"
            type="text"
            inputmode="decimal"
            bind:value={withholdingStr}
            placeholder={formatEditableAmount('0', 2)}
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted"
          />
        </div>
        <div>
          <label for="div-withholding-account" class="mb-1 block text-sm font-medium text-foreground">
            {m.investments_form_withholding_account()}
          </label>
          <select
            id="div-withholding-account"
            bind:value={withholdingAccountID}
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
          >
            <option value="">{m.investments_form_select_account()}</option>
            {#each withholdingAccounts as acc (acc.id)}
              <option value={String(acc.id)}>{acc.name}</option>
            {/each}
          </select>
        </div>
      </div>
    {/if}
  {/if}

  <!-- Memo -->
  <div>
    <label for="div-memo" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_memo()}
    </label>
    <input
      id="div-memo"
      type="text"
      bind:value={memo}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
    />
  </div>

  <APIFormError error={formError} id="div-form-error" />

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
      disabled={!canSubmit || pending}
      class="rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
    >
      {submitLabel}
    </button>
  </div>
</form>
