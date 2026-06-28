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
    previewSell,
    recordSell,
    type InvestmentInstrumentResponse,
    type SellPreviewResponse,
    type CostBasisMethod
  } from '$lib/api/investments';
  import { formatScaledValue, costBasisMethodLabel } from '$lib/investments/investment-labels';
  import { getLocale } from '$lib/paraglide/runtime.js';

  let {
    csrfToken,
    onSaved,
    onCancel
  }: {
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
  } = $props();

  const locale = getLocale();
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
    clearPreview();
  }

  // Form fields
  let transactionDate = $state(todayISO());
  let holdingAccountID = $state('');
  let cashAccountID = $state('');
  let quantityStr = $state('');
  let cashAmountStr = $state('');
  let costBasisMethod = $state<CostBasisMethod>('fifo');
  let memo = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);

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

  const cashCommodityID = $derived(selectedCashAccount?.default_commodity_id);
  const cashCurrencyCode = $derived(
    cashCommodityID ? (currenciesByID.get(cashCommodityID)?.code ?? '') : ''
  );

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

  const previewReady = $derived(
    !!selectedInstrument &&
    holdingAccountID !== '' &&
    cashAccountID !== '' &&
    !!cashCommodityID &&
    quantityStr.trim() !== '' &&
    cashAmountStr.trim() !== ''
  );

  const canSubmit = $derived(previewReady && preview !== null && !previewPending);

  // Trigger preview debounced when inputs change
  $effect(() => {
    // Track all reactive deps
    const _inst = selectedInstrument?.commodity_id;
    const _hold = holdingAccountID;
    const _cash = cashAccountID;
    const _qty = quantityStr;
    const _amt = cashAmountStr;
    const _method = costBasisMethod;
    const _comm = cashCommodityID;

    clearPreview();
    clearTimeout(previewDebounceTimer);

    if (!previewReady) return;

    previewDebounceTimer = setTimeout(() => {
      void triggerPreview();
    }, 400);
  });

  async function triggerPreview() {
    if (!selectedInstrument || !cashCommodityID) return;

    const qty = parseDecimalField(quantityStr);
    const cash = parseDecimalField(cashAmountStr);
    if (qty.value === null || cash.value === null) return;

    const qtyInt = toSafeInt(qty.value);
    const cashInt = toSafeInt(cash.value);
    if (qtyInt === null || cashInt === null) {
      previewError = new Error(m.investments_form_amount_too_large());
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
        quantity_value: qtyInt,
        quantity_scale: qty.scale,
        cash_amount_value: cashInt,
        cash_amount_scale: cash.scale,
        cash_commodity_id: cashCommodityID,
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
    if (!canSubmit || !selectedInstrument || !cashCommodityID) return;

    const qty = parseDecimalField(quantityStr);
    const cash = parseDecimalField(cashAmountStr);
    if (qty.value === null || cash.value === null) return;

    const qtyInt = toSafeInt(qty.value);
    const cashInt = toSafeInt(cash.value);
    if (qtyInt === null || cashInt === null) {
      formError = new Error(m.investments_form_amount_too_large());
      return;
    }

    pending = true;
    formError = undefined;

    try {
      await recordSell(
        {
          transaction_date: transactionDate,
          commodity_id: selectedInstrument.commodity_id,
          holding_account_id: Number(holdingAccountID),
          cash_account_id: Number(cashAccountID),
          quantity_value: qtyInt,
          quantity_scale: qty.scale,
          cash_amount_value: cashInt,
          cash_amount_scale: cash.scale,
          cash_commodity_id: cashCommodityID,
          cost_basis_method: costBasisMethod,
          memo: memo.trim() || undefined
        },
        csrfToken
      );

      await queryClient.invalidateQueries({ queryKey: investmentPositionsQueryKey });
      await queryClient.invalidateQueries({ queryKey: investmentLotsQueryKey });
      onSaved();
    } catch (err) {
      formError = err;
    } finally {
      pending = false;
    }
  }

  // specific_lot excluded until lot-allocation picker UI is added (requires per-lot selection UI)
  const COST_BASIS_METHODS: CostBasisMethod[] = ['fifo', 'lifo', 'average_cost'];

  function formatGain(gain: number, scale: number): string {
    return formatScaledValue(gain, scale, locale);
  }
</script>

<form onsubmit={handleSubmit} class="space-y-4">
  <h2 class="text-base font-semibold text-foreground">{m.investments_sell_title()}</h2>

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
      {#each COST_BASIS_METHODS as method (method)}
        <option value={method}>{costBasisMethodLabel(method)}</option>
      {/each}
    </select>
  </div>

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
  {#if previewPending}
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
        <span class="text-muted">{m.investments_sell_preview_proceeds()}</span>
        <span class="text-right font-mono text-foreground">
          {formatGain(preview.cash_amount_value, preview.cash_amount_scale)}
          {#if cashCurrencyCode}<span class="ml-1 text-muted">{cashCurrencyCode}</span>{/if}
        </span>
        <span class="text-muted">{m.investments_sell_preview_gain()}</span>
        <span class="text-right font-mono {preview.realized_gain >= 0 ? 'text-foreground' : 'text-destructive'}">
          {preview.realized_gain >= 0 ? '+' : ''}{formatGain(preview.realized_gain, preview.cash_amount_scale)}
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
      disabled={!canSubmit || pending}
      class="rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
    >
      {pending ? m.investments_sell_pending() : m.investments_sell_submit()}
    </button>
  </div>
</form>
