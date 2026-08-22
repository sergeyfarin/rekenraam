<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { m } from '$lib/paraglide/messages.js';
  import { accountsQueryOptions, type AccountResponse } from '$lib/api/accounts';
  import { currenciesQueryOptions, type CurrencyResponse } from '$lib/api/currencies';
  import {
    investmentPositionsQueryKey,
    investmentLotsQueryKey,
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
  } from '$lib/api/investments';
  import ReconciliationConfirm from '$lib/investments/reconciliation-confirm.svelte';

  let {
    mode = 'cash',
    csrfToken,
    onSaved,
    onCancel
  }: {
    mode?: 'cash' | 'reinvested';
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
  } = $props();

  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
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
    applyDefaultsForInstrument(inst);
  }

  // Form fields
  let transactionDate = $state(todayISO());
  // cash dividend only
  let cashAccountID = $state('');
  let incomeAccountID = $state('');
  let withholdingAccountID = $state('');
  let amountStr = $state('');
  let withholdingStr = $state('');
  let showWithholding = $state(false);
  // reinvested only
  let holdingAccountID = $state('');
  let quantityStr = $state('');
  let memo = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);

  // Both dividend shapes can be backdated into a reconciled period, so both get
  // the same named confirmation as the trade forms (T-47). The pending payload
  // is tagged so the confirm handler knows which route to retry.
  type PendingDividend =
    | { mode: 'cash'; payload: DividendRequest }
    | { mode: 'reinvest'; payload: ReinvestedDividendRequest };
  let reconciliationModal = $state<{
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    pending: PendingDividend;
  } | null>(null);

  function todayISO(): string {
    return new Date().toISOString().slice(0, 10);
  }

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

  const holdingAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) =>
        (a.account_kind === 'security_holding' || a.account_kind === 'fund_holding') &&
        a.status === 'active'
    )
  );

  const incomeAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) => a.account_class === 'income' && a.status === 'active' && a.allows_postings
    )
  );

  const liabilityAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) => a.account_class === 'liability' && a.status === 'active' && a.allows_postings
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

  const cashCommodityID = $derived(selectedCashAccount?.default_commodity_id);
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
      withholdingStr = rawLedgerToDisplay(String(match.default_withholding_value), match.default_withholding_scale);
      showWithholding = true;
    }
  }

  function rawLedgerToDisplay(quantityValue: string, scale: number): string {
    const abs = quantityValue.startsWith('-') ? quantityValue.slice(1) : quantityValue;
    const isNeg = quantityValue.startsWith('-');
    if (scale === 0) return isNeg ? `-${abs}` : abs;
    if (abs.length <= scale) {
      const padded = abs.padStart(scale + 1, '0');
      const int = padded.slice(0, padded.length - scale);
      const frac = padded.slice(padded.length - scale);
      return isNeg ? `-${int}.${frac}` : `${int}.${frac}`;
    }
    const int = abs.slice(0, abs.length - scale);
    const frac = abs.slice(abs.length - scale);
    return isNeg ? `-${int}.${frac}` : `${int}.${frac}`;
  }

  function parseDecimalField(s: string): { value: bigint | null; scale: number } {
    const trimmed = s.trim().replace(/,/g, '');
    if (!trimmed) return { value: null, scale: 0 };
    const dotIdx = trimmed.indexOf('.');
    let intStr: string;
    let fracStr: string;
    let scale: number;
    if (dotIdx === -1) {
      intStr = trimmed || '0';
      fracStr = '';
      scale = 0;
    } else {
      intStr = trimmed.slice(0, dotIdx) || '0';
      fracStr = trimmed.slice(dotIdx + 1);
      scale = fracStr.length;
    }
    intStr = intStr.replace(/^0+/, '') || '0';
    const coefficient = intStr + fracStr;
    if (!/^\d+$/.test(coefficient)) return { value: null, scale };
    return { value: BigInt(coefficient), scale };
  }

  function toSafeInt(v: bigint): number | null {
    if (v > BigInt(Number.MAX_SAFE_INTEGER)) return null;
    return Number(v);
  }

  const canSubmitCash = $derived(
    cashAccountID !== '' &&
    !!cashCommodityID &&
    incomeAccountID !== '' &&
    amountStr.trim() !== ''
  );

  const canSubmitReinvested = $derived(
    !!selectedInstrument &&
    holdingAccountID !== '' &&
    cashAccountID !== '' &&
    !!cashCommodityID &&
    quantityStr.trim() !== '' &&
    amountStr.trim() !== ''
  );

  const canSubmit = $derived(mode === 'cash' ? canSubmitCash : canSubmitReinvested);

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!canSubmit || !cashCommodityID) return;

    const amount = parseDecimalField(amountStr);
    if (amount.value === null) return;

    const amountInt = toSafeInt(amount.value);
    if (amountInt === null) {
      formError = new Error(m.investments_form_amount_too_large());
      return;
    }

    pending = true;
    formError = undefined;

    try {
      if (mode === 'cash') {
        const withholding = showWithholding && withholdingStr.trim() ? parseDecimalField(withholdingStr) : { value: null, scale: 0 };
        const withholdingInt = withholding.value !== null ? toSafeInt(withholding.value) : null;
        if (withholding.value !== null && withholdingInt === null) {
          formError = new Error(m.investments_form_amount_too_large());
          pending = false;
          return;
        }
        const payload: DividendRequest = {
          transaction_date: transactionDate,
          commodity_id: selectedInstrument?.commodity_id,
          cash_account_id: Number(cashAccountID),
          cash_commodity_id: cashCommodityID,
          income_account_id: incomeAccountID ? Number(incomeAccountID) : undefined,
          amount_value: amountInt,
          amount_scale: amount.scale,
          withholding_value: withholdingInt !== null ? withholdingInt : undefined,
          withholding_scale: withholdingInt !== null ? withholding.scale : undefined,
          withholding_account_id:
            withholdingInt !== null && withholdingAccountID
              ? Number(withholdingAccountID)
              : undefined,
          memo: memo.trim() || undefined
        };

        const impact = await dividendReconciliationImpact(payload);
        if (impact.affected_checkpoints.length > 0) {
          reconciliationModal = {
            impacts: impact.affected_checkpoints,
            pending: { mode: 'cash', payload }
          };
          return;
        }

        await recordDividend(payload, csrfToken);
      } else {
        if (!selectedInstrument) return;
        const qty = parseDecimalField(quantityStr);
        if (qty.value === null) return;
        const qtyInt = toSafeInt(qty.value);
        if (qtyInt === null) {
          formError = new Error(m.investments_form_amount_too_large());
          pending = false;
          return;
        }

        const payload: ReinvestedDividendRequest = {
          transaction_date: transactionDate,
          commodity_id: selectedInstrument.commodity_id,
          holding_account_id: Number(holdingAccountID),
          income_account_id: incomeAccountID ? Number(incomeAccountID) : undefined,
          quantity_value: qtyInt,
          quantity_scale: qty.scale,
          amount_value: amountInt,
          amount_scale: amount.scale,
          cash_commodity_id: cashCommodityID,
          memo: memo.trim() || undefined
        };

        const impact = await reinvestedDividendReconciliationImpact(payload);
        if (impact.affected_checkpoints.length > 0) {
          reconciliationModal = {
            impacts: impact.affected_checkpoints,
            pending: { mode: 'reinvest', payload }
          };
          return;
        }

        await recordReinvestedDividend(payload, csrfToken);
      }

      await refreshAfterSave();
    } catch (err) {
      formError = err;
    } finally {
      pending = false;
    }
  }

  async function confirmOverride() {
    if (!reconciliationModal) return;
    const target = reconciliationModal.pending;
    reconciliationModal = null;
    pending = true;
    formError = undefined;

    try {
      if (target.mode === 'cash') {
        await recordDividend({ ...target.payload, reconciliation_override: true }, csrfToken);
      } else {
        await recordReinvestedDividend(
          { ...target.payload, reconciliation_override: true },
          csrfToken
        );
      }

      await refreshAfterSave();
    } catch (err) {
      formError = err;
    } finally {
      pending = false;
    }
  }

  async function refreshAfterSave() {
    await queryClient.invalidateQueries({ queryKey: investmentPositionsQueryKey });
    await queryClient.invalidateQueries({ queryKey: investmentLotsQueryKey });
    onSaved();
  }

  const title = $derived(mode === 'cash' ? m.investments_dividend_title() : m.investments_reinvested_title());
  const submitLabel = $derived(
    pending
      ? (mode === 'cash' ? m.investments_dividend_pending() : m.investments_reinvested_pending())
      : (mode === 'cash' ? m.investments_dividend_submit() : m.investments_reinvested_submit())
  );
</script>

{#if reconciliationModal}
  <ReconciliationConfirm
    impacts={reconciliationModal.impacts}
    {pending}
    onCancel={() => (reconciliationModal = null)}
    onConfirm={confirmOverride}
  />
{/if}

<form onsubmit={handleSubmit} class="space-y-4">
  <h2 class="text-base font-semibold text-foreground">{title}</h2>

  <!-- Date -->
  <div>
    <label for="div-date" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_date()}
    </label>
    <input
      id="div-date"
      type="date"
      bind:value={transactionDate}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
      required
    />
  </div>

  <!-- Instrument autocomplete (optional for cash dividend, required for reinvested) -->
  <div class="relative">
    <label for="div-instrument" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_instrument()}
      {#if mode === 'cash'}
        <span class="ml-1 text-xs font-normal text-muted">({m.investments_form_optional()})</span>
      {/if}
    </label>
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
        {#each holdingAccounts as acc (acc.id)}
          <option value={String(acc.id)}>{acc.name}</option>
        {/each}
      </select>
    </div>
  {/if}

  <!-- Cash account -->
  <div>
    <label for="div-cash-account" class="mb-1 block text-sm font-medium text-foreground">
      {mode === 'cash' ? m.investments_form_cash_account() : m.investments_form_currency_account()}
    </label>
    <select
      id="div-cash-account"
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
        placeholder="0.000"
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted"
        required
      />
    </div>
  {/if}

  <!-- Amount -->
  <div>
    <label for="div-amount" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_dividend_amount()}
      {#if cashCurrencyCode}
        <span class="ml-1 text-xs font-normal text-muted">({cashCurrencyCode})</span>
      {/if}
    </label>
    <input
      id="div-amount"
      type="text"
      inputmode="decimal"
      bind:value={amountStr}
      placeholder="0.00"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted"
      required
    />
  </div>

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
            placeholder="0.00"
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
            {#each liabilityAccounts as acc (acc.id)}
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
      class="rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
    >
      {submitLabel}
    </button>
  </div>
</form>
