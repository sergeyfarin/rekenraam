<script lang="ts">
  import { useSettlementCandidates } from '#lib/investments/settlement-candidates.svelte.ts';
  import { isInvestmentHoldingAccount } from '#lib/investments/settlement-accounts.ts';
  import { investmentEntryAccountsQueryOptions } from '#lib/api/investment-entry.ts';
  import { autofocus } from '#lib/components/autofocus.ts';
  import { untrack } from 'svelte';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { commitReview, type CommitReviewState } from '#lib/investments/commit-review.ts';
  import type { ReviewGuards } from '#lib/investments/preview-review.ts';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import type { AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryOptions,
    paymentInLieuReconciliationImpact,
    previewPaymentInLieuReplacementReconciliation,
    recordPaymentInLieu,
    replacePaymentInLieu,
    type InvestmentCorrectionPaymentInLieuTerms,
    type PaymentInLieuRequest
  } from '#lib/api/investments.ts';
  import { formatEditableAmount } from '#lib/money/amount.ts';
  import { parseDividendAmounts, type AmountFieldError } from '#lib/investments/form-amounts.ts';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import type { InvestmentPrefill } from '#lib/investments/investment-prefill.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import { isSettlementAccount } from './settlement-accounts.ts';

  // Cash is required; optional attribution is editable through the same
  // audited replacement as amount/account corrections. A correction may also
  // change the payment date, cash account and, with the account, currency (#395).
  let {
    csrfToken,
    onSaved,
    onCancel,
    prefill,
    correction
  }: {
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
    prefill?: InvestmentPrefill;
    correction?: { transactionID: number; terms: InvestmentCorrectionPaymentInLieuTerms };
  } = $props();
  const entryPrefill = untrack(() => correction ? undefined : prefill);
  const initialPrefill = untrack(() => prefill);
  const initial = untrack(() => correction);
  const terms = initial?.terms;
  const ownerToday = useOwnerToday();

  const queryClient = useQueryClient();

  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());

  let holdingAccountID = $state(terms ? (terms.holding_account_id ? String(terms.holding_account_id) : '')
    : initialPrefill?.holdingAccountID !== undefined ? String(initialPrefill.holdingAccountID) : '');
  let commodityID = $state(terms ? (terms.commodity_id ? String(terms.commodity_id) : '')
    : initialPrefill?.commodityID !== undefined ? String(initialPrefill.commodityID) : '');
  let exDate = $state(terms?.ex_date ?? '');
  let paymentDate = $state(terms?.event_date ?? entryPrefill?.transactionDate ?? ownerToday());
  let cashAccountID = $state(terms ? String(terms.cash_account_id) : '');
  let expenseAccountID = $state(terms ? String(terms.expense_account_id) : '');
  let amountStr = $state(terms ? formatEditableAmount(terms.amount_value, terms.amount_scale) : '');
  let showRecovery = $state(terms?.recovery_value !== undefined);
  let recoveryStr = $state(terms?.recovery_value !== undefined && terms.recovery_scale !== undefined
    ? formatEditableAmount(terms.recovery_value, terms.recovery_scale) : '');
  let recoveryAccountID = $state(terms?.recovery_account_id ? String(terms.recovery_account_id) : '');
  let memo = $state(terms?.memo ?? entryPrefill?.memo ?? '');
  let reason = $state('');
  let pending = $state(false);
  const accountsQuery = createQuery(() => investmentEntryAccountsQueryOptions(paymentDate || ownerToday()));
  let formError = $state<unknown>(undefined);
  let review = $state.raw<CommitReviewState<PaymentInLieuRequest> | null>(null);


  const accounts = $derived(accountsQuery.data?.accounts ?? []);
  const instruments = $derived(instrumentsQuery.data?.instruments ?? []);
  const holdingAccounts = $derived(accounts.filter((account: AccountResponse) =>
    isInvestmentHoldingAccount(account)));
  const cashAccounts = $derived(accounts.filter((account: AccountResponse) =>
    isSettlementAccount(account, terms?.cash_account_id)));
  const expenseAccounts = $derived(accounts.filter((account: AccountResponse) =>
    account.account_class === 'expense' && account.allows_postings &&
    (account.status === 'active' || account.id === terms?.expense_account_id)));
  // Matches the backend recovery role: the same expense, income, or a
  // receivable or liability.
  const recoveryAccounts = $derived(accounts.filter((account: AccountResponse) =>
    ['asset', 'liability', 'expense', 'income'].includes(account.account_class) && account.allows_postings && !account.system_role &&
    !isInvestmentHoldingAccount(account) && (account.status === 'active' || account.id === terms?.recovery_account_id)));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency: CurrencyResponse) => [currency.id, currency])));
  // The currency is the cash account's; a correction to an account without a
  // default commodity keeps the recorded currency.
  const cashCommodityID = $derived(
    accounts.find((account: AccountResponse) => String(account.id) === cashAccountID)?.default_commodity_id ??
      terms?.cash_commodity_id ?? undefined);
  const currencyCode = $derived(cashCommodityID ? currenciesByID.get(cashCommodityID)?.code ?? '' : '');
  const loading = $derived(accountsQuery.isPending || currenciesQuery.isPending || instrumentsQuery.isPending);
  const loadError = $derived(accountsQuery.isError || currenciesQuery.isError || instrumentsQuery.isError);
  const canSubmit = $derived(!loading && !loadError && !!paymentDate &&
    cashAccounts.some((account) => String(account.id) === cashAccountID) && !!cashCommodityID && expenseAccountID !== '' && amountStr.trim() !== '' &&
    (!showRecovery || recoveryStr.trim() === '' || recoveryAccountID !== '') && (!initial || !!reason.trim()));

  function amountErrorMessage(error: AmountFieldError): string {
    switch (error) {
      case 'negative':
        return m.investments_form_negative_number();
      case 'too_large':
        return m.investments_form_amount_too_large();
      case 'invalid':
        return m.investments_form_invalid_number();
    }
  }

  async function handleSubmit(event: Event) {
    event.preventDefault();
    if (!canSubmit || pending || !cashCommodityID) return;
    const amounts = parseDividendAmounts({ amountStr, withholdingStr: recoveryStr, includeWithholding: showRecovery,
      quantityStr: '', includeQuantity: false });
    if (!amounts.ok) {
      formError = new Error(amountErrorMessage(amounts.reason));
      return;
    }
    const { amount, withholding: recovery } = amounts.values;
    const payload: PaymentInLieuRequest = {
      holding_account_id: holdingAccountID ? Number(holdingAccountID) : null, commodity_id: commodityID ? Number(commodityID) : null, ex_date: exDate || null,
      transaction_date: paymentDate, cash_account_id: Number(cashAccountID), cash_commodity_id: cashCommodityID,
      expense_account_id: Number(expenseAccountID), amount_value: amount.value, amount_scale: amount.scale,
      recovery_value: recovery?.value, recovery_scale: recovery?.scale,
      recovery_account_id: recovery && recoveryAccountID ? Number(recoveryAccountID) : undefined,
      memo: memo.trim() || undefined, payee_id: terms?.payee_id
    };
    pending = true;
    formError = undefined;
    try {
      if (await impactReview.open(payload, false)) return;
      await commit(payload, {});
    } catch (error) {
      formError = error;
    } finally {
      pending = false;
    }
  }

  async function commit(payload: PaymentInLieuRequest, guards: ReviewGuards) {
    if (initial) {
      await replacePaymentInLieu(initial.transactionID,
        { reason: reason.trim(), replacement: payload, ...guards }, csrfToken);
    } else {
      await recordPaymentInLieu({ ...payload, ...guards }, csrfToken);
    }
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  // Show the checkpoints the write would invalidate, if any, for an override.
  // Checkpoint-only: this flow never acquires a gain requirement (#400).
  const impactReview = commitReview({
    protocol: 'checkpoints',
    get review() { return review; },
    impact: (payload: PaymentInLieuRequest) => initial
      ? previewPaymentInLieuReplacementReconciliation(initial.transactionID, { reason: reason.trim(), replacement: payload })
      : paymentInLieuReconciliationImpact(payload),
    write: commit,
    setReview: (value) => { review = value; },
    setPending: (value) => { pending = value; },
    setError: (error) => { formError = error; }
  });

  const inputClass = 'min-h-11 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground';
  const chooseSettlement = useSettlementCandidates(() => accountsQuery.data?.accounts,
    () => Number(holdingAccountID) || undefined, () => cashAccountID, (id) => { cashAccountID = id; }, entryPrefill, untrack(() => !!correction));
</script>

{#if review}
  <ReconciliationConfirm impacts={review.impacts} gainRows={[]} gainRefreshed={false} {pending}
    onCancel={impactReview.cancel} onConfirm={impactReview.confirm} />
{/if}

<form onsubmit={handleSubmit} aria-busy={pending} class="space-y-4" aria-describedby="pil-copy">
  <h2 id="payment-in-lieu-title" class="text-base font-semibold text-foreground">
    {initial ? m.investments_payment_in_lieu_correct_title() : m.investments_payment_in_lieu_title()}
  </h2>
  <p id="pil-copy" class="text-sm text-muted">{m.investments_payment_in_lieu_copy()}</p>

  {#if loading}
    <p class="text-sm text-muted" role="status">{m.investments_payment_in_lieu_loading()}</p>
  {:else if loadError}
    <p class="text-sm text-danger" role="alert">{m.investments_payment_in_lieu_load_error()}</p>
  {/if}

  <fieldset class="space-y-3 rounded-(--radius-panel) border border-border p-3">
    <legend class="px-1 text-sm font-medium text-foreground">{m.investments_attribution_optional()}</legend>
    <p class="text-xs text-muted">{m.investments_attribution_repair_hint()}</p>
    <div>
      <label for="pil-holding" class="mb-1 block text-sm font-medium text-foreground">{m.investments_payment_in_lieu_holding()}</label>
      <select id="pil-holding" bind:value={holdingAccountID} class={inputClass}>
        <option value="">{m.investments_attribution_unassigned()}</option>
        {#if accountsQuery.data && holdingAccountID && !holdingAccounts.some(account => String(account.id) === holdingAccountID)}
          <option value={holdingAccountID}>#{holdingAccountID}</option>
        {/if}
        {#each holdingAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
      </select>
    </div>
    <div>
      <label for="pil-instrument" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_instrument()}</label>
      <select id="pil-instrument" bind:value={commodityID} class={inputClass}>
        <option value="">{m.investments_attribution_unassigned()}</option>
        {#if commodityID && !instruments.some(instrument => String(instrument.commodity_id) === commodityID)}
          <option value={commodityID}>#{commodityID}</option>
        {/if}
        {#each instruments as instrument (instrument.commodity_id)}
          <option value={String(instrument.commodity_id)}>{instrument.symbol ?? instrument.commodity_code} · {instrument.display_name}</option>
        {/each}
      </select>
    </div>
    <div>
      <label for="pil-ex-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_payment_in_lieu_ex_date()}</label>
      <input id="pil-ex-date" type="date" bind:value={exDate} class={inputClass} aria-describedby="pil-ex-date-hint" />
      <p id="pil-ex-date-hint" class="mt-1 text-xs text-muted">{m.investments_payment_in_lieu_ex_date_hint()}</p>
    </div>
  </fieldset>

  {#if initial}
    <div>
      <label for="pil-reason" class="mb-1 block text-sm font-medium text-foreground">{m.investments_payment_in_lieu_correction_reason()}</label>
      <input id="pil-reason" type="text" {@attach autofocus} bind:value={reason} maxlength="500" required class={inputClass} />
    </div>
  {/if}
  <div>
    <label for="pil-payment-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_payment_in_lieu_payment_date()}</label>
    <input id="pil-payment-date" type="date" bind:value={paymentDate} required class={inputClass} />
  </div>
  <div>
    <label for="pil-cash" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_cash_account()}</label>
    <select onchange={chooseSettlement} id="pil-cash" bind:value={cashAccountID} required class={inputClass}>
      <option value="">{m.investments_form_select_account()}</option>
      {#if cashAccountID && !cashAccounts.some((account) => String(account.id) === cashAccountID)}
        <option value={cashAccountID} disabled>{(accountsQuery.data?.accounts ?? []).find((account) => String(account.id) === cashAccountID)?.name ?? cashAccountID}</option>
      {/if}
      {#each cashAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
    </select>
    <p class="mt-1 text-xs text-muted">{cashAccountID ? m.investments_activity_settlement_hint() : m.investments_activity_settlement_choice()}</p>
    {#if accountsQuery.data && cashAccountID && !cashAccounts.some((account) => String(account.id) === cashAccountID)}
      <p role="alert" class="mt-1 text-sm text-danger">{m.investments_activity_settlement_invalid()}</p>
    {/if}
  </div>

  <div>
    <label for="pil-amount" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_payment_in_lieu_amount()}
      {#if currencyCode}<span class="ml-1 text-xs font-normal text-muted">({currencyCode})</span>{/if}
    </label>
    <input id="pil-amount" type="text" inputmode="decimal" bind:value={amountStr} placeholder={formatEditableAmount('0', 2)} required
      class="{inputClass} font-mono placeholder:text-muted" />
  </div>
  <div>
    <label for="pil-expense" class="mb-1 block text-sm font-medium text-foreground">{m.investments_payment_in_lieu_expense_account()}</label>
    <select id="pil-expense" bind:value={expenseAccountID} required class={inputClass}>
      <option value="">{m.investments_form_select_account()}</option>
      {#each expenseAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
    </select>
  </div>

  <label class="flex min-h-11 items-center gap-2 text-sm font-medium text-foreground">
    <input type="checkbox" bind:checked={showRecovery} class="rounded" />
    {m.investments_payment_in_lieu_recovery_toggle()}
  </label>
  {#if showRecovery}
    <div class="space-y-3 rounded-(--radius-panel) border border-border p-3">
      <div>
        <label for="pil-recovery" class="mb-1 block text-sm font-medium text-foreground">
          {m.investments_payment_in_lieu_recovery_amount()}
          {#if currencyCode}<span class="ml-1 text-xs font-normal text-muted">({currencyCode})</span>{/if}
        </label>
        <input id="pil-recovery" type="text" inputmode="decimal" bind:value={recoveryStr} placeholder={formatEditableAmount('0', 2)}
          class="{inputClass} font-mono placeholder:text-muted" />
      </div>
      <div>
        <label for="pil-recovery-account" class="mb-1 block text-sm font-medium text-foreground">{m.investments_payment_in_lieu_recovery_account()}</label>
        <select id="pil-recovery-account" bind:value={recoveryAccountID} class={inputClass}>
          <option value="">{m.investments_form_select_account()}</option>
          {#each recoveryAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
        </select>
      </div>
    </div>
  {/if}

  <div>
    <label for="pil-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
    <input id="pil-memo" type="text" bind:value={memo} class={inputClass} />
  </div>

  <APIFormError error={formError} id="pil-form-error" />

  <div class="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
    <button type="button" onclick={onCancel}
      class="min-h-11 rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    <button type="submit" disabled={!canSubmit || pending}
      class="min-h-11 rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
      {pending ? m.investments_payment_in_lieu_pending() : initial ? m.investments_payment_in_lieu_correct_submit() : m.investments_payment_in_lieu_submit()}
    </button>
  </div>
</form>
