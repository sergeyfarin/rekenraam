<script lang="ts">
  import { untrack } from 'svelte';
  import { commitReview, type CommitReviewState } from '#lib/investments/commit-review.ts';
  import { createQuery } from '@tanstack/svelte-query';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { accountsQueryOptions, type AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import { isSubledgerManagedAccountKind } from '#lib/accounts/subledger-accounts.ts';
  import { formatEditableAmount } from '#lib/money/amount.ts';
  import { parseDividendAmounts, type AmountFieldError } from '#lib/investments/form-amounts.ts';
  import {
    acceptEventSuggestion,
    paymentInLieuReconciliationImpact,
    type EventSuggestionCompletion
  } from '#lib/api/investments.ts';
  import { grossPrefill, type PaymentInLieuProposal, type ProviderDividendEvidence } from './provider-dividend.ts';

  // Completes a provider dividend that found the book short (#255) into a
  // payment in lieu: the owner supplies the payment date, cash account,
  // amount owed and expense account. It posts through the native command.
  let {
    suggestionID,
    proposal,
    evidence,
    csrfToken,
    onPosted,
    onCancel
  }: {
    suggestionID: number;
    proposal: PaymentInLieuProposal;
    evidence: ProviderDividendEvidence | null;
    csrfToken: string;
    onPosted: () => Promise<void> | void;
    onCancel: () => void;
  } = $props();
  const initial = untrack(() => proposal);

  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let paymentDate = $state(untrack(() => evidence?.exDate ?? initial.exDate));
  let holdingAccountID = $state(initial.holdingAccountIDs.length === 1 ? String(initial.holdingAccountIDs[0]) : '');
  let cashAccountID = $state('');
  let expenseAccountID = $state('');
  let amountStr = $state('');
  let recoveryStr = $state('');
  let recoveryAccountID = $state('');
  let showRecovery = $state(false);
  let memo = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let prefilled = $state(false);
  let reconciliation = $state.raw<CommitReviewState<EventSuggestionCompletion> | null>(null);

  const accounts = $derived(accountsQuery.data?.accounts ?? []);
  const accountName = (id: number) => accounts.find((account: AccountResponse) => account.id === id)?.name ?? `#${id}`;
  const cashAccounts = $derived(accounts.filter((a: AccountResponse) =>
    ['asset', 'liability'].includes(a.account_class) && a.status === 'active' && a.allows_postings && !a.system_role &&
    !isSubledgerManagedAccountKind(a.account_kind)));
  const expenseAccounts = $derived(accounts.filter((a: AccountResponse) => a.account_class === 'expense' && a.status === 'active' && a.allows_postings));
  const recoveryAccounts = $derived(accounts.filter((a: AccountResponse) =>
    ['asset', 'liability', 'expense', 'income'].includes(a.account_class) && !a.system_role &&
    !isSubledgerManagedAccountKind(a.account_kind) && a.status === 'active' && a.allows_postings));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>((currenciesQuery.data?.currencies ?? []).map((c: CurrencyResponse) => [c.id, c])));
  const cashCommodityID = $derived(cashAccounts.find((a: AccountResponse) => String(a.id) === cashAccountID)?.default_commodity_id ?? undefined);
  const cashCurrency = $derived(cashCommodityID ? currenciesByID.get(cashCommodityID) : undefined);

  // The estimate (short units × per-share amount) is offered once in the
  // listing currency; the owner confirms it against the broker statement.
  $effect(() => {
    if (!cashCurrency || amountStr !== '' || !evidence) return;
    const estimate = grossPrefill(evidence.estimatedGross, evidence.listingCurrency, cashCurrency.code, cashCurrency.standard_scale);
    if (estimate) {
      untrack(() => {
        amountStr = formatEditableAmount(estimate.value, estimate.scale);
        prefilled = true;
      });
    }
  });

  const canSubmit = $derived(paymentDate !== '' && holdingAccountID !== '' && cashAccountID !== '' && !!cashCommodityID &&
    expenseAccountID !== '' && amountStr.trim() !== '' && (!showRecovery || recoveryStr.trim() === '' || recoveryAccountID !== '') && !pending);

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
    const amounts = parseDividendAmounts({ amountStr, withholdingStr: recoveryStr, includeWithholding: showRecovery, quantityStr: '', includeQuantity: false });
    if (!amounts.ok) {
      formError = new Error(amountErrorMessage(amounts.reason));
      return;
    }
    const { amount, withholding: recovery } = amounts.values;
    const completion: EventSuggestionCompletion = {
      transaction_date: paymentDate,
      cash_account_id: Number(cashAccountID),
      cash_commodity_id: cashCommodityID,
      amount_value: amount.value,
      amount_scale: amount.scale,
      holding_account_id: Number(holdingAccountID),
      expense_account_id: Number(expenseAccountID),
      recovery_value: recovery?.value,
      recovery_scale: recovery?.scale,
      recovery_account_id: recovery && recoveryAccountID ? Number(recoveryAccountID) : undefined,
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

  // Checkpoint-only: this flow never acquires a gain requirement (#400).
  const impactReview = commitReview({
    protocol: 'checkpoints',
    get review() { return reconciliation; },
    impact: (completion: EventSuggestionCompletion) => paymentInLieuReconciliationImpact({
      holding_account_id: Number(holdingAccountID), commodity_id: initial.commodityID, ex_date: initial.exDate,
      transaction_date: paymentDate, cash_account_id: completion.cash_account_id, cash_commodity_id: completion.cash_commodity_id,
      expense_account_id: Number(expenseAccountID), amount_value: completion.amount_value, amount_scale: completion.amount_scale,
      recovery_value: completion.recovery_value, recovery_scale: completion.recovery_scale,
      recovery_account_id: completion.recovery_account_id, memo: completion.memo
    }),
    write: (completion, guards) => post({ ...completion, ...guards }),
    setReview: (value) => { reconciliation = value; },
    setPending: (value) => { pending = value; },
    setError: (error) => { formError = error; }
  });

  const fieldClass = 'min-h-11 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground';
  const idPrefix = $derived(`suggestion-${suggestionID}`);
</script>

{#if reconciliation}
  <ReconciliationConfirm impacts={reconciliation.impacts} gainRows={[]} gainRefreshed={false} {pending}
    onCancel={impactReview.cancel} onConfirm={impactReview.confirm} />
{/if}

<form onsubmit={handleSubmit} aria-busy={pending} class="mt-3 space-y-3 rounded-(--radius-panel) border border-border p-3" aria-labelledby="{idPrefix}-complete-title">
  <h3 id="{idPrefix}-complete-title" class="text-sm font-semibold text-foreground">{m.investments_suggestion_payment_in_lieu_complete_title()}</h3>
  <div class="grid gap-3 sm:grid-cols-2">
    {#if initial.holdingAccountIDs.length > 1}
      <div class="sm:col-span-2">
        <label for="{idPrefix}-holding" class="mb-1 block text-sm font-medium text-foreground">{m.investments_payment_in_lieu_holding()}</label>
        <select id="{idPrefix}-holding" bind:value={holdingAccountID} required class={fieldClass}>
          <option value="">{m.investments_form_select_account()}</option>
          {#each initial.holdingAccountIDs as id (id)}<option value={String(id)}>{accountName(id)}</option>{/each}
        </select>
      </div>
    {/if}
    <div>
      <label for="{idPrefix}-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_suggestion_payment_date()}</label>
      <input id="{idPrefix}-date" type="date" bind:value={paymentDate} min={initial.exDate} required class={fieldClass} />
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
        {m.investments_payment_in_lieu_amount()}
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
      <label for="{idPrefix}-expense" class="mb-1 block text-sm font-medium text-foreground">{m.investments_payment_in_lieu_expense_account()}</label>
      <select id="{idPrefix}-expense" bind:value={expenseAccountID} required class={fieldClass}>
        <option value="">{m.investments_form_select_account()}</option>
        {#each expenseAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
      </select>
    </div>
  </div>
  <label class="flex min-h-11 items-center gap-2 text-sm font-medium text-foreground">
    <input type="checkbox" bind:checked={showRecovery} class="rounded" />
    {m.investments_payment_in_lieu_recovery_toggle()}
  </label>
  {#if showRecovery}
    <div class="grid gap-3 sm:grid-cols-2">
      <div>
        <label for="{idPrefix}-recovery" class="mb-1 block text-sm font-medium text-foreground">{m.investments_payment_in_lieu_recovery_amount()}</label>
        <input id="{idPrefix}-recovery" type="text" inputmode="decimal" bind:value={recoveryStr}
          placeholder={formatEditableAmount('0', 2)} class="{fieldClass} font-mono placeholder:text-muted" />
      </div>
      <div>
        <label for="{idPrefix}-recovery-account" class="mb-1 block text-sm font-medium text-foreground">{m.investments_payment_in_lieu_recovery_account()}</label>
        <select id="{idPrefix}-recovery-account" bind:value={recoveryAccountID} class={fieldClass}>
          <option value="">{m.investments_form_select_account()}</option>
          {#each recoveryAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
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
      class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-1.5 text-sm font-semibold text-foreground transition hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    <button type="submit" disabled={!canSubmit}
      class="min-h-11 rounded-(--radius-control) bg-accent px-3 py-1.5 text-sm font-semibold text-accent-foreground transition hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-50">
      {pending ? m.investments_suggestion_posting() : m.investments_suggestion_post()}
    </button>
  </div>
</form>
