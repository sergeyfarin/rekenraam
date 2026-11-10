<script lang="ts">
  import { untrack } from 'svelte';
  import { commitReview, type CommitReviewState } from '#lib/investments/commit-review.ts';
  import { createQuery } from '@tanstack/svelte-query';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { accountsQueryOptions, type AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import { formatEditableAmount } from '#lib/money/amount.ts';
  import { parseDividendAmounts, type AmountFieldError } from '#lib/investments/form-amounts.ts';
  import {
    acceptEventSuggestion,
    dividendReconciliationImpact,
    type EventSuggestionCompletion
  } from '#lib/api/investments.ts';
  import { grossPrefill, type ProviderDividendEvidence } from './provider-dividend.ts';

  let {
    suggestionID,
    commodityID,
    evidence,
    csrfToken,
    onPosted,
    onCancel
  }: {
    suggestionID: number;
    commodityID: number | undefined;
    evidence: ProviderDividendEvidence | null;
    csrfToken: string;
    onPosted: () => Promise<void> | void;
    onCancel: () => void;
  } = $props();

  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  // The payment date is unknown to the provider; start from the ex-date the
  // owner can correct, never from a guessed payment date.
  let paymentDate = $state(untrack(() => evidence?.exDate ?? ''));
  let cashAccountID = $state('');
  let incomeAccountID = $state('');
  let withholdingAccountID = $state('');
  let amountStr = $state('');
  let withholdingStr = $state('');
  let showWithholding = $state(false);
  let memo = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let prefilled = $state(false);
  let reconciliation = $state.raw<CommitReviewState<EventSuggestionCompletion> | null>(null);

  const accounts = $derived(accountsQuery.data?.accounts ?? []);
  const cashAccounts = $derived(accounts.filter((a: AccountResponse) =>
    a.account_class === 'asset' && a.status === 'active' && a.allows_postings &&
    a.account_kind !== 'security_holding' && a.account_kind !== 'fund_holding'));
  const incomeAccounts = $derived(accounts.filter((a: AccountResponse) => a.account_class === 'income' && a.status === 'active' && a.allows_postings));
  const withholdingAccounts = $derived(accounts.filter((a: AccountResponse) =>
    ['asset', 'liability', 'expense'].includes(a.account_class) && !a.system_role &&
    !['security_holding', 'fund_holding'].includes(a.account_kind) && a.status === 'active' && a.allows_postings));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>((currenciesQuery.data?.currencies ?? []).map((c: CurrencyResponse) => [c.id, c])));
  const cashCommodityID = $derived(cashAccounts.find((a: AccountResponse) => String(a.id) === cashAccountID)?.default_commodity_id ?? undefined);
  const cashCurrency = $derived(cashCommodityID ? currenciesByID.get(cashCommodityID) : undefined);

  // Choosing a cash account in the listing currency offers the estimate once;
  // the owner still confirms it against the statement.
  $effect(() => {
    if (!cashCurrency || amountStr !== '' || !evidence) return;
    const prefill = grossPrefill(evidence.estimatedGross, evidence.listingCurrency, cashCurrency.code, cashCurrency.standard_scale);
    if (prefill) {
      untrack(() => {
        amountStr = formatEditableAmount(prefill.value, prefill.scale);
        prefilled = true;
      });
    }
  });

  const canSubmit = $derived(paymentDate !== '' && cashAccountID !== '' && !!cashCommodityID && amountStr.trim() !== '' && !pending);

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

  async function post(completion: EventSuggestionCompletion) {
    await acceptEventSuggestion(suggestionID, csrfToken, completion);
    await onPosted();
  }

  async function handleSubmit(event: Event) {
    event.preventDefault();
    if (!canSubmit || !cashCommodityID) return;
    const amounts = parseDividendAmounts({ amountStr, withholdingStr, includeWithholding: showWithholding, quantityStr: '', includeQuantity: false });
    if (!amounts.ok) {
      formError = new Error(amountErrorMessage(amounts.reason));
      return;
    }
    const { amount, withholding } = amounts.values;
    const completion: EventSuggestionCompletion = {
      transaction_date: paymentDate,
      cash_account_id: Number(cashAccountID),
      cash_commodity_id: cashCommodityID,
      amount_value: amount.value,
      amount_scale: amount.scale,
      income_account_id: incomeAccountID ? Number(incomeAccountID) : undefined,
      withholding_value: withholding?.value,
      withholding_scale: withholding?.scale,
      withholding_account_id: withholding && withholdingAccountID ? Number(withholdingAccountID) : undefined,
      memo: memo.trim() || undefined
    };
    pending = true;
    formError = undefined;
    try {
      if (await impactReview.open(completion, false)) return;
      await post(completion);
    } catch (err) {
      formError = err;
    } finally {
      pending = false;
    }
  }

  // A backdated payment can land in a reconciled period: the same named
  // confirmation as manual dividend entry.
  // Checkpoint-only: this flow never acquires a gain requirement (#400).
  const impactReview = commitReview({
    protocol: 'checkpoints',
    get review() { return reconciliation; },
    impact: (completion: EventSuggestionCompletion) => dividendReconciliationImpact({
      transaction_date: completion.transaction_date,
      withholding_only: false,
      commodity_id: commodityID,
      cash_account_id: completion.cash_account_id,
      cash_commodity_id: completion.cash_commodity_id,
      income_account_id: completion.income_account_id,
      amount_value: completion.amount_value,
      amount_scale: completion.amount_scale,
      withholding_value: completion.withholding_value,
      withholding_scale: completion.withholding_scale,
      withholding_account_id: completion.withholding_account_id,
      memo: completion.memo
    }),
    write: (completion, guards) => post({ ...completion, ...guards }),
    setReview: (value) => { reconciliation = value; },
    setPending: (value) => { pending = value; },
    setError: (error) => { formError = error; }
  });

  const fieldClass = 'w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground';
  const idPrefix = $derived(`suggestion-${suggestionID}`);
</script>

{#if reconciliation}
  <ReconciliationConfirm impacts={reconciliation.impacts} gainRows={[]} gainRefreshed={false} {pending}
    onCancel={impactReview.cancel} onConfirm={impactReview.confirm} />
{/if}

<form onsubmit={handleSubmit} aria-busy={pending} class="mt-3 space-y-3 rounded-(--radius-panel) border border-border p-3" aria-labelledby="{idPrefix}-complete-title">
  <h3 id="{idPrefix}-complete-title" class="text-sm font-semibold text-foreground">{m.investments_suggestion_complete_title()}</h3>
  <div class="grid gap-3 sm:grid-cols-2">
    <div>
      <label for="{idPrefix}-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_suggestion_payment_date()}</label>
      <input id="{idPrefix}-date" type="date" bind:value={paymentDate} required class={fieldClass} />
    </div>
    <div>
      <label for="{idPrefix}-cash" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_cash_account()}</label>
      <select id="{idPrefix}-cash" bind:value={cashAccountID} required class={fieldClass}>
        <option value="">{m.investments_form_select_account()}</option>
        {#each cashAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
      </select>
    </div>
    <div>
      <label for="{idPrefix}-amount" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_suggestion_received_gross()}
        {#if cashCurrency}<span class="ml-1 text-xs font-normal text-muted">({cashCurrency.code})</span>{/if}
      </label>
      <input id="{idPrefix}-amount" type="text" inputmode="decimal" bind:value={amountStr} required
        placeholder={formatEditableAmount('0', 2)} aria-describedby={prefilled ? `${idPrefix}-estimate-hint` : undefined}
        class="{fieldClass} font-mono placeholder:text-muted" />
      {#if prefilled}
        <p id="{idPrefix}-estimate-hint" class="mt-1 text-xs text-muted">{m.investments_suggestion_estimate_hint()}</p>
      {/if}
    </div>
    <div>
      <label for="{idPrefix}-income" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_form_income_account()}
        <span class="ml-1 text-xs font-normal text-muted">({m.investments_form_optional()})</span>
      </label>
      <select id="{idPrefix}-income" bind:value={incomeAccountID} class={fieldClass}>
        <option value="">{m.investments_form_select_account()}</option>
        {#each incomeAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
      </select>
    </div>
  </div>
  <label class="flex items-center gap-2 text-sm font-medium text-foreground">
    <input type="checkbox" bind:checked={showWithholding} class="rounded" />
    {m.investments_form_withholding_toggle()}
  </label>
  {#if showWithholding}
    <div class="grid gap-3 sm:grid-cols-2">
      <div>
        <label for="{idPrefix}-withholding" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_withholding_amount()}</label>
        <input id="{idPrefix}-withholding" type="text" inputmode="decimal" bind:value={withholdingStr}
          placeholder={formatEditableAmount('0', 2)} class="{fieldClass} font-mono placeholder:text-muted" />
      </div>
      <div>
        <label for="{idPrefix}-withholding-account" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_withholding_account()}</label>
        <select id="{idPrefix}-withholding-account" bind:value={withholdingAccountID} class={fieldClass}>
          <option value="">{m.investments_form_select_account()}</option>
          {#each withholdingAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
        </select>
      </div>
    </div>
  {/if}
  <div>
    <label for="{idPrefix}-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
    <input id="{idPrefix}-memo" type="text" bind:value={memo} class={fieldClass} />
  </div>
  <APIFormError error={formError} id="{idPrefix}-error" />
  <div class="flex flex-wrap justify-end gap-2">
    <button type="button" onclick={onCancel}
      class="rounded-(--radius-control) border border-border bg-control px-3 py-1.5 text-sm font-semibold text-foreground transition hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    <button type="submit" disabled={!canSubmit}
      class="rounded-(--radius-control) bg-accent px-3 py-1.5 text-sm font-semibold text-accent-foreground transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
      {pending ? m.investments_suggestion_posting() : m.investments_suggestion_post()}
    </button>
  </div>
</form>
