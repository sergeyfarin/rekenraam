<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { m } from '$lib/paraglide/messages.js';
  import { parseTradeAmounts, type AmountFieldError } from '$lib/investments/form-amounts';
  import { accountsQueryOptions, type AccountResponse } from '$lib/api/accounts';
  import { currenciesQueryOptions, type CurrencyResponse } from '$lib/api/currencies';
  import {
    investmentPositionsQueryKey,
    investmentLotsQueryKey,
    investmentInstrumentsQueryKey,
    searchInvestmentInstruments,
    recordBuy,
    buyReconciliationImpact,
    type InvestmentInstrumentResponse,
    type InvestmentTradeRequest,
    type ReconciliationImpactResponse
  } from '$lib/api/investments';
  import ReconciliationConfirm from '$lib/investments/reconciliation-confirm.svelte';

  let {
    csrfToken,
    onSaved,
    onCancel
  }: {
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
  } = $props();

  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

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
  }

  // Form fields
  let transactionDate = $state(todayISO());
  let holdingAccountID = $state('');
  let cashAccountID = $state('');
  let quantityStr = $state('');
  let cashAmountStr = $state('');
  let memo = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);

  // A backdated buy can land inside a reconciled period. Rather than letting
  // the server refuse it with no way forward, preview the impact and let the
  // user accept the named consequences (T-47).
  let reconciliationModal = $state<{
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    payload: InvestmentTradeRequest;
  } | null>(null);

  function todayISO(): string {
    return new Date().toISOString().slice(0, 10);
  }

  // Holding accounts: security_holding or fund_holding kinds
  const holdingAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) =>
        (a.account_kind === 'security_holding' || a.account_kind === 'fund_holding') &&
        a.status === 'active'
    )
  );

  // Cash accounts: asset accounts that allow postings (except holding accounts)
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

  const cashCommodityID = $derived(selectedCashAccount?.default_commodity_id);
  const cashCurrencyCode = $derived(
    cashCommodityID ? (currenciesByID.get(cashCommodityID)?.code ?? '') : ''
  );

  // Amount validation lives in $lib/investments/form-amounts.ts so its
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

  const canSubmit = $derived(
    !!selectedInstrument &&
    holdingAccountID !== '' &&
    cashAccountID !== '' &&
    !!cashCommodityID &&
    quantityStr.trim() !== '' &&
    cashAmountStr.trim() !== ''
  );

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!canSubmit || !selectedInstrument || !cashCommodityID) return;

    // quantity_value is an exact coefficient string on the wire, so it needs
    // no safe-integer cap; cash_amount_value is a real int64 and still does.
    const amounts = parseTradeAmounts({ quantityStr, cashAmountStr });
    if (!amounts.ok) {
      formError = new Error(amountErrorMessage(amounts.reason));
      return;
    }
    const { quantity, cashAmount } = amounts.values;

    const payload: InvestmentTradeRequest = {
      transaction_date: transactionDate,
      commodity_id: selectedInstrument.commodity_id,
      holding_account_id: Number(holdingAccountID),
      cash_account_id: Number(cashAccountID),
      quantity_value: quantity.value,
      quantity_scale: quantity.scale,
      cash_amount_value: cashAmount.int64,
      cash_amount_scale: cashAmount.scale,
      cash_commodity_id: cashCommodityID,
      memo: memo.trim() || undefined
    };

    pending = true;
    formError = undefined;

    try {
      const impact = await buyReconciliationImpact(payload);
      if (impact.affected_checkpoints.length > 0) {
        // Hand the decision to the user rather than overriding for them.
        reconciliationModal = { impacts: impact.affected_checkpoints, payload };
        return;
      }

      await submitBuy(payload, false);
    } catch (err) {
      formError = err;
    } finally {
      pending = false;
    }
  }

  async function confirmOverride() {
    if (!reconciliationModal) return;
    const { payload } = reconciliationModal;
    reconciliationModal = null;
    pending = true;
    formError = undefined;

    try {
      await submitBuy(payload, true);
    } catch (err) {
      formError = err;
    } finally {
      pending = false;
    }
  }

  async function submitBuy(payload: InvestmentTradeRequest, override: boolean) {
    await recordBuy(override ? { ...payload, reconciliation_override: true } : payload, csrfToken);

    await queryClient.invalidateQueries({ queryKey: investmentPositionsQueryKey });
    await queryClient.invalidateQueries({ queryKey: investmentLotsQueryKey });
    onSaved();
  }
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
  <h2 class="text-base font-semibold text-foreground">{m.investments_buy_title()}</h2>

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
      {#each holdingAccounts as acc (acc.id)}
        <option value={String(acc.id)}>{acc.name}</option>
      {/each}
    </select>
  </div>

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
      placeholder="0.000"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted"
      required
    />
  </div>

  <!-- Cash account -->
  <div>
    <label for="buy-cash-account" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_cash_account()}
    </label>
    <select
      id="buy-cash-account"
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
      placeholder="0.00"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted"
      required
    />
  </div>

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
      disabled={!canSubmit || pending}
      class="rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
    >
      {pending ? m.investments_buy_pending() : m.investments_buy_submit()}
    </button>
  </div>
</form>
