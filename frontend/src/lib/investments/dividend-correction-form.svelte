<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import { untrack } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { accountsQueryOptions, type AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import { formatLedgerAmount } from '#lib/money/amount.ts';
  import { parseDividendAmounts, type AmountFieldError } from '#lib/investments/form-amounts.ts';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import {
    gainAcknowledgement,
    gainImpactCurrency,
    gainImpactRows,
    hasGainChanges,
    impactNeedsReview,
    isGainAcknowledgementRefusal
  } from '#lib/investments/gain-impact.ts';
  import {
    previewDividendReplacementReconciliation,
    previewReinvestmentReplacementReconciliation,
    replaceDividend,
    replaceReinvestedDividend,
    type DividendRequest,
    type GainImpact,
    type InvestmentCorrectionDividendTerms,
    type InvestmentCorrectionReinvestmentTerms,
    type ReconciliationImpactResponse,
    type ReinvestedDividendRequest
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';

  // Replaces a posted cash dividend or reinvested dividend (T-115). Only the
  // amounts, withholding, quantity, income account and memo are editable: the
  // date, account and currency are fixed, so the form shows them read-only.
  let {
    csrfToken,
    transactionID,
    dividend,
    reinvestment,
    onSaved,
    onCancel
  }: {
    csrfToken: string;
    transactionID: number;
    dividend?: InvestmentCorrectionDividendTerms;
    reinvestment?: InvestmentCorrectionReinvestmentTerms;
    onSaved: () => void;
    onCancel: () => void;
  } = $props();

  const initialDividend = untrack(() => dividend);
  const initialReinvestment = untrack(() => reinvestment);
  const reinvested = initialReinvestment !== undefined;
  const terms = (initialReinvestment ?? initialDividend)!;

  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let reason = $state('');
  let amountStr = $state(formatLedgerAmount(terms.amount_value, terms.amount_scale));
  let quantityStr = $state(initialReinvestment
    ? formatLedgerAmount(initialReinvestment.quantity_value, initialReinvestment.quantity_scale) : '');
  let incomeAccountID = $state(String(terms.income_account_id));
  let showWithholding = $state(initialDividend?.withholding_value !== undefined);
  let withholdingStr = $state(initialDividend?.withholding_value !== undefined && initialDividend.withholding_scale !== undefined
    ? formatLedgerAmount(initialDividend.withholding_value, initialDividend.withholding_scale) : '');
  let withholdingAccountID = $state(initialDividend?.withholding_account_id ? String(initialDividend.withholding_account_id) : '');
  let memo = $state(terms.memo ?? '');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let reasonInputElement: HTMLInputElement | undefined = $state();

  $effect(() => {
    reasonInputElement?.focus();
  });

  type Pending =
    | { kind: 'cash'; payload: DividendRequest }
    | { kind: 'reinvested'; payload: ReinvestedDividendRequest };
  let review = $state<{
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    gainRefreshed: boolean;
    pending: Pending;
  } | null>(null);

  const accounts = $derived(accountsQuery.data?.accounts ?? []);
  const accountName = (id: number) => accounts.find((account: AccountResponse) => account.id === id)?.name ?? `#${id}`;
  const incomeAccounts = $derived(accounts.filter((account: AccountResponse) =>
    account.account_class === 'income' && ((account.status === 'active' && account.allows_postings) || account.id === terms.income_account_id)));
  // The original withholding account stays selectable even when it is not a
  // liability, so an unchanged replacement never silently moves the tax.
  const withholdingAccounts = $derived(accounts.filter((account: AccountResponse) =>
    (account.account_class === 'liability' && account.status === 'active' && account.allows_postings) ||
    account.id === initialDividend?.withholding_account_id));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency: CurrencyResponse) => [currency.id, currency])));
  const currencyCode = $derived(currenciesByID.get(terms.cash_commodity_id)?.code ?? '');
  const gainRows = $derived(review?.gainImpact
    ? gainImpactRows(review.gainImpact.changes, gainImpactCurrency(currenciesByID), getLocale()) : []);
  const dateLabel = $derived(new Intl.DateTimeFormat(getLocale(), { year: 'numeric', month: 'short', day: 'numeric' })
    .format(parseISO(terms.event_date)));
  const canSubmit = $derived(!!reason.trim() && amountStr.trim() !== '' && incomeAccountID !== '' &&
    (!reinvested || quantityStr.trim() !== '') &&
    (!showWithholding || withholdingStr.trim() === '' || withholdingAccountID !== ''));

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
    if (!canSubmit || pending) return;
    const amounts = parseDividendAmounts({
      amountStr, withholdingStr, includeWithholding: !reinvested && showWithholding,
      quantityStr, includeQuantity: reinvested
    });
    if (!amounts.ok) {
      formError = new Error(amountErrorMessage(amounts.reason));
      return;
    }
    const { amount, withholding, quantity } = amounts.values;
    let target: Pending;
    if (initialReinvestment) {
      if (!quantity) return;
      target = { kind: 'reinvested', payload: {
        transaction_date: initialReinvestment.event_date, commodity_id: initialReinvestment.commodity_id,
        holding_account_id: initialReinvestment.holding_account_id, cash_commodity_id: initialReinvestment.cash_commodity_id,
        income_account_id: Number(incomeAccountID), quantity_value: quantity.value, quantity_scale: quantity.scale,
        amount_value: amount.value, amount_scale: amount.scale, memo: memo.trim() || undefined,
        payee_id: initialReinvestment.payee_id
      } };
    } else {
      target = { kind: 'cash', payload: {
        transaction_date: terms.event_date, cash_account_id: initialDividend!.cash_account_id,
        cash_commodity_id: terms.cash_commodity_id, income_account_id: Number(incomeAccountID),
        amount_value: amount.value, amount_scale: amount.scale,
        withholding_value: withholding?.value, withholding_scale: withholding?.scale,
        withholding_account_id: withholding && withholdingAccountID ? Number(withholdingAccountID) : undefined,
        memo: memo.trim() || undefined, payee_id: initialDividend!.payee_id
      } };
    }
    pending = true;
    formError = undefined;
    try {
      if (!(await reviewImpact(target, false))) await commit(target, false, '');
    } catch (error) {
      await recover(error, target);
    } finally {
      pending = false;
    }
  }

  // Preview through the actual replacement writer; checkpoints or changed
  // gains go to the confirmation instead of committing.
  async function reviewImpact(target: Pending, refreshed: boolean): Promise<boolean> {
    const body = { reason: reason.trim() };
    const impact = target.kind === 'reinvested'
      ? await previewReinvestmentReplacementReconciliation(transactionID, { ...body, replacement: target.payload })
      : await previewDividendReplacementReconciliation(transactionID, { ...body, replacement: target.payload });
    if (!impactNeedsReview(impact)) return false;
    const gainImpact = hasGainChanges(impact.gain_impact) ? impact.gain_impact : null;
    review = { impacts: impact.affected_checkpoints, gainImpact, gainRefreshed: refreshed && !!gainImpact, pending: target };
    return true;
  }

  async function commit(target: Pending, override: boolean, acknowledgement: string) {
    const body = { reason: reason.trim(), ...(override ? { reconciliation_override: true } : {}) };
    if (target.kind === 'reinvested') {
      await replaceReinvestedDividend(transactionID, { ...body, replacement: target.payload,
        ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {}) }, csrfToken);
    } else {
      await replaceDividend(transactionID, { ...body, replacement: target.payload }, csrfToken);
    }
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  // The gain set changed since review: show the current set, not a dead end.
  async function recover(error: unknown, target: Pending) {
    try {
      if (!isGainAcknowledgementRefusal(error) || !(await reviewImpact(target, true))) formError = error;
    } catch (previewError) {
      formError = previewError;
    }
  }

  async function confirmReview() {
    if (!review) return;
    const { pending: target, impacts, gainImpact } = review;
    review = null;
    pending = true;
    formError = undefined;
    try {
      await commit(target, impacts.length > 0, gainAcknowledgement(gainImpact));
    } catch (error) {
      await recover(error, target);
    } finally {
      pending = false;
    }
  }
</script>

{#if review}
  <ReconciliationConfirm impacts={review.impacts} gainRows={gainRows} gainRefreshed={review.gainRefreshed}
    {pending} onCancel={() => (review = null)} onConfirm={confirmReview} />
{/if}

<form onsubmit={handleSubmit} class="space-y-4">
  <h2 class="text-base font-semibold text-foreground">
    {reinvested ? m.transactions_investment_replace_reinvestment_title() : m.transactions_investment_replace_dividend_title()}
  </h2>
  <p class="text-sm text-muted">
    {reinvested ? m.transactions_investment_replace_reinvestment_copy() : m.transactions_investment_replace_dividend_copy()}
  </p>

  <div>
    <label for="dividend-correction-reason" class="mb-1 block text-sm font-medium text-foreground">
      {m.transactions_investment_replace_reason()}
    </label>
    <input id="dividend-correction-reason" type="text" bind:this={reasonInputElement} bind:value={reason}
      maxlength="500" required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <dl class="grid grid-cols-1 gap-2 rounded-(--radius-panel) border border-border p-3 text-sm sm:grid-cols-2">
    <div>
      <dt class="text-xs text-muted">{m.investments_form_date()}</dt>
      <dd class="font-medium text-foreground">{dateLabel}</dd>
    </div>
    <div>
      <dt class="text-xs text-muted">{reinvested ? m.investments_form_holding_account() : m.investments_form_cash_account()}</dt>
      <dd class="font-medium text-foreground">
        {accountName(initialReinvestment ? initialReinvestment.holding_account_id : initialDividend!.cash_account_id)}
        {#if currencyCode}<span class="text-muted"> · {currencyCode}</span>{/if}
      </dd>
    </div>
  </dl>

  {#if reinvested}
    <div>
      <label for="dividend-correction-quantity" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_form_quantity()}
      </label>
      <input id="dividend-correction-quantity" type="text" inputmode="decimal" bind:value={quantityStr} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground" />
    </div>
  {/if}

  <div>
    <label for="dividend-correction-amount" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_dividend_amount()}
      {#if currencyCode}<span class="ml-1 text-xs font-normal text-muted">({currencyCode})</span>{/if}
    </label>
    <input id="dividend-correction-amount" type="text" inputmode="decimal" bind:value={amountStr} required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground" />
  </div>

  <div>
    <label for="dividend-correction-income" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_income_account()}
    </label>
    <select id="dividend-correction-income" bind:value={incomeAccountID} required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
      {#each incomeAccounts as account (account.id)}
        <option value={String(account.id)}>{account.name}</option>
      {/each}
    </select>
  </div>

  {#if !reinvested}
    <label class="flex items-center gap-2 text-sm font-medium text-foreground">
      <input type="checkbox" bind:checked={showWithholding} class="h-4 w-4 rounded" />
      {m.investments_form_withholding_toggle()}
    </label>
    {#if showWithholding}
      <div class="space-y-3 rounded-(--radius-panel) border border-border p-3">
        <div>
          <label for="dividend-correction-withholding" class="mb-1 block text-sm font-medium text-foreground">
            {m.investments_form_withholding_amount()}
            {#if currencyCode}<span class="ml-1 text-xs font-normal text-muted">({currencyCode})</span>{/if}
          </label>
          <input id="dividend-correction-withholding" type="text" inputmode="decimal" bind:value={withholdingStr}
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground" />
        </div>
        <div>
          <label for="dividend-correction-withholding-account" class="mb-1 block text-sm font-medium text-foreground">
            {m.investments_form_withholding_account()}
          </label>
          <select id="dividend-correction-withholding-account" bind:value={withholdingAccountID}
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
            <option value="">{m.investments_form_select_account()}</option>
            {#each withholdingAccounts as account (account.id)}
              <option value={String(account.id)}>{account.name}</option>
            {/each}
          </select>
        </div>
      </div>
    {/if}
  {/if}

  <div>
    <label for="dividend-correction-memo" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_memo()}
    </label>
    <input id="dividend-correction-memo" type="text" bind:value={memo}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <APIFormError error={formError} id="dividend-correction-error" />

  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel} disabled={pending}
      class="min-h-10 rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    <button type="submit" disabled={!canSubmit || pending}
      class="min-h-10 rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
      {m.transactions_investment_replace_submit()}
    </button>
  </div>
</form>
