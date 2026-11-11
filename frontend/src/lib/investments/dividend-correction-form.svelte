<script lang="ts">
  import { autofocus } from '#lib/components/autofocus.ts';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { accountsQueryOptions, type AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import { investmentInstrumentsQueryOptions } from '#lib/api/investments.ts';
  import { formatEditableAmount } from '#lib/money/amount.ts';
  import { parseDividendAmounts, type AmountFieldError } from '#lib/investments/form-amounts.ts';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { gainImpactCurrency, gainImpactRows } from '#lib/investments/gain-impact.ts';
  import { commitReview, type CommitReviewState } from '#lib/investments/commit-review.ts';
  import type { ReviewGuards } from '#lib/investments/preview-review.ts';
  import {
    previewDividendReplacementReconciliation,
    previewReinvestmentReplacementReconciliation,
    replaceDividend,
    replaceReinvestedDividend,
    type DividendRequest,
    type InvestmentCorrectionDividendTerms,
    type InvestmentCorrectionReinvestmentTerms,
    type ReinvestedDividendRequest
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import DividendSourceDifferences from './dividend-source-differences.svelte';
  import { isSettlementAccount } from './settlement-accounts.ts';

  // Replaces a posted cash dividend or reinvested dividend (T-115). A cash
  // dividend's date, cash account and currency are correctable (#395), with
  // the currency following the cash account as on entry; its payment shape is
  // fixed. A reinvestment's date, holding, instrument and cost currency are
  // correctable through old/new-position replay; the gain review names any
  // later sale it restates.
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
  const withholdingOnly = initialDividend?.withholding_only === true;
  const terms = (initialReinvestment ?? initialDividend)!;

  const queryClient = useQueryClient();
  // Archived accounts too: a recorded account that has since been archived
  // must stay selected rather than silently fall back to another option.
  const accountsQuery = createQuery(() => accountsQueryOptions(true, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());

  let reason = $state('');
  let amountStr = $state(formatEditableAmount(terms.amount_value, terms.amount_scale));
  let quantityStr = $state(initialReinvestment
    ? formatEditableAmount(initialReinvestment.quantity_value, initialReinvestment.quantity_scale) : '');
  let incomeAccountID = $state(String(terms.income_account_id));
  let showWithholding = $state(initialDividend?.withholding_value !== undefined);
  let withholdingStr = $state(initialDividend?.withholding_value !== undefined && initialDividend.withholding_scale !== undefined
    ? formatEditableAmount(initialDividend.withholding_value, initialDividend.withholding_scale) : '');
  let withholdingAccountID = $state(initialDividend?.withholding_account_id ? String(initialDividend.withholding_account_id) : '');
  let memo = $state(terms.memo ?? '');
  // A cash dividend's payer may be assigned or changed by replacement (#244).
  // Omission keeps the recorded payer, so "unassigned" is offered only when
  // none was recorded.
  let payerID = $state(initialDividend?.commodity_id ? String(initialDividend.commodity_id) : '');
  // The ex date is always sent: a blank field clears a recorded one.
  let exDate = $state(initialDividend?.ex_date ?? '');
  let transactionDate = $state(terms.event_date);
  let cashAccountID = $state(initialDividend ? String(initialDividend.cash_account_id) : '');
  let holdingAccountID = $state(initialReinvestment ? String(initialReinvestment.holding_account_id) : '');
  let instrumentCommodityID = $state(initialReinvestment ? String(initialReinvestment.commodity_id) : '');
  let costCommodityID = $state(initialReinvestment ? String(initialReinvestment.cash_commodity_id) : '');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);


  type Pending =
    | { kind: 'cash'; payload: DividendRequest }
    | { kind: 'reinvested'; payload: ReinvestedDividendRequest };
  let review = $state<CommitReviewState<Pending> | null>(null);

  const accounts = $derived(accountsQuery.data?.accounts ?? []);
  const incomeAccounts = $derived(accounts.filter((account: AccountResponse) =>
    account.account_class === 'income' && ((account.status === 'active' && account.allows_postings) || account.id === terms.income_account_id)));
  // Match the backend tax roles; retain the original account even if archived.
  const withholdingAccounts = $derived(accounts.filter((account: AccountResponse) =>
    (['asset', 'liability', 'expense'].includes(account.account_class) && !account.system_role &&
      !['security_holding', 'fund_holding'].includes(account.account_kind) && account.status === 'active' && account.allows_postings) ||
    account.id === initialDividend?.withholding_account_id));
  // The backend settlement role, retaining the recorded account even if archived.
  const cashAccounts = $derived(accounts.filter((account: AccountResponse) =>
    isSettlementAccount(account, initialDividend?.cash_account_id)));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency: CurrencyResponse) => [currency.id, currency])));
  // As on entry, the currency is the cash account's; an account without a
  // default commodity keeps the recorded currency.
  const cashCommodityID = $derived(initialDividend
    ? (cashAccounts.find((account: AccountResponse) => String(account.id) === cashAccountID)?.default_commodity_id ??
      initialDividend.cash_commodity_id)
    : Number(costCommodityID));
  // The holdings a reinvestment may open its lot in, retaining the recorded one.
  const holdingAccounts = $derived(accounts.filter((account: AccountResponse) =>
    (['security_holding', 'fund_holding'].includes(account.account_kind) && account.status === 'active') ||
    account.id === initialReinvestment?.holding_account_id));
  const currencyCode = $derived(currenciesByID.get(cashCommodityID)?.code ?? '');
  const gainRows = $derived(review?.gainImpact
    ? gainImpactRows(review.gainImpact.changes, gainImpactCurrency(currenciesByID), getLocale()) : []);
  const canSubmit = $derived(!!reason.trim() && amountStr.trim() !== '' && transactionDate !== '' &&
    (reinvested ? holdingAccountID !== '' && instrumentCommodityID !== '' && costCommodityID !== '' : cashAccountID !== '') && (withholdingOnly ? withholdingAccountID !== '' : incomeAccountID !== '') &&
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
      amountStr, withholdingStr, includeWithholding: !reinvested && !withholdingOnly && showWithholding,
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
        transaction_date: transactionDate, commodity_id: Number(instrumentCommodityID),
        holding_account_id: Number(holdingAccountID), cash_commodity_id: Number(costCommodityID),
        income_account_id: Number(incomeAccountID), quantity_value: quantity.value, quantity_scale: quantity.scale,
        amount_value: amount.value, amount_scale: amount.scale, memo: memo.trim() || undefined,
        payee_id: initialReinvestment.payee_id
      } };
    } else {
      target = { kind: 'cash', payload: {
        commodity_id: payerID ? Number(payerID) : null,
        ex_date: exDate || null,
        withholding_only: withholdingOnly,
        transaction_date: transactionDate, cash_account_id: Number(cashAccountID),
        cash_commodity_id: cashCommodityID, income_account_id: withholdingOnly ? undefined : Number(incomeAccountID),
        amount_value: amount.value, amount_scale: amount.scale,
        withholding_value: withholding?.value, withholding_scale: withholding?.scale,
        withholding_account_id: (withholdingOnly || withholding) && withholdingAccountID ? Number(withholdingAccountID) : undefined,
        memo: memo.trim() || undefined, payee_id: initialDividend!.payee_id
      } };
    }
    pending = true;
    formError = undefined;
    try {
      if (!(await impactReview.open(target, false))) await commit(target, {});
    } catch (error) {
      await impactReview.recover(error, target);
    } finally {
      pending = false;
    }
  }

  // Only a reinvestment can change committed gains; a cash replacement sends
  // the reviewed checkpoints alone.
  async function commit(target: Pending, guards: ReviewGuards) {
    const { gain_impact_acknowledgement: acknowledgement, ...checkpoints } = guards;
    const body = { reason: reason.trim(), ...checkpoints };
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
  const impactReview = commitReview({
    protocol: 'gains',
    get review() { return review; },
    impact: (target: Pending) => target.kind === 'reinvested'
      ? previewReinvestmentReplacementReconciliation(transactionID, { reason: reason.trim(), replacement: target.payload })
      : previewDividendReplacementReconciliation(transactionID, { reason: reason.trim(), replacement: target.payload }),
    write: commit,
    setReview: (value) => { review = value; },
    setPending: (value) => { pending = value; },
    setError: (error) => { formError = error; }
  });
</script>

{#if review}
  <ReconciliationConfirm impacts={review.impacts} gainRows={gainRows} gainRefreshed={review.gainRefreshed}
    {pending} onCancel={impactReview.cancel} onConfirm={impactReview.confirm} />
{/if}

<form onsubmit={handleSubmit} aria-busy={pending} class="space-y-4">
  <h2 class="text-base font-semibold text-foreground">
    {withholdingOnly ? m.investments_withholding_correct_title() : reinvested ? m.transactions_investment_replace_reinvestment_title() : m.transactions_investment_replace_dividend_title()}
  </h2>
  <p class="text-sm text-muted">
    {withholdingOnly ? m.investments_withholding_correct_copy() : reinvested ? m.transactions_investment_replace_reinvestment_copy() : m.transactions_investment_replace_dividend_copy()}
  </p>

  <div>
    <label for="dividend-correction-reason" class="mb-1 block text-sm font-medium text-foreground">
      {m.transactions_investment_replace_reason()}
    </label>
    <input id="dividend-correction-reason" type="text" {@attach autofocus} bind:value={reason}
      maxlength="500" required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  {#if initialDividend}<DividendSourceDifferences differences={initialDividend.source_differences} />{/if}

  <div>
    <!-- A cash dividend or tax payment is dated by its payment; a reinvestment by its acquisition. -->
    <label for="dividend-correction-date" class="mb-1 block text-sm font-medium text-foreground">
      {reinvested ? m.investments_form_date() : m.investments_form_payment_date()}
    </label>
    <input id="dividend-correction-date" type="date" bind:value={transactionDate} required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>
  {#if initialReinvestment}
    <div>
      <label for="dividend-correction-holding" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_holding_account()}</label>
      <select id="dividend-correction-holding" bind:value={holdingAccountID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {#each holdingAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
      </select>
    </div>
    <div>
      <label for="dividend-correction-instrument" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_instrument()}</label>
      <select id="dividend-correction-instrument" bind:value={instrumentCommodityID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {#if !(instrumentsQuery.data?.instruments ?? []).some((instrument) => String(instrument.commodity_id) === instrumentCommodityID)}
          <option value={instrumentCommodityID}>#{instrumentCommodityID}</option>
        {/if}
        {#each instrumentsQuery.data?.instruments ?? [] as instrument (instrument.commodity_id)}
          <option value={String(instrument.commodity_id)}>{instrument.display_name} ({instrument.symbol || instrument.commodity_code})</option>
        {/each}
      </select>
    </div>
    <div>
      <label for="dividend-correction-currency" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_currency()}</label>
      <select id="dividend-correction-currency" bind:value={costCommodityID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {#if !currenciesByID.has(Number(costCommodityID))}<option value={costCommodityID}>#{costCommodityID}</option>{/if}
        {#each [...currenciesByID.values()] as currency (currency.id)}<option value={String(currency.id)}>{currency.code}</option>{/each}
      </select>
    </div>
  {:else}
    <div>
      <label for="dividend-correction-cash-account" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_form_cash_account()}
        {#if currencyCode}<span class="ml-1 text-xs font-normal text-muted">({currencyCode})</span>{/if}
      </label>
      <select id="dividend-correction-cash-account" bind:value={cashAccountID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {#each cashAccounts as account (account.id)}
          <option value={String(account.id)}>{account.name}</option>
        {/each}
      </select>
    </div>
  {/if}

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
      {withholdingOnly ? m.investments_form_withholding_amount() : m.investments_form_dividend_amount()}
      {#if currencyCode}<span class="ml-1 text-xs font-normal text-muted">({currencyCode})</span>{/if}
    </label>
    <input id="dividend-correction-amount" type="text" inputmode="decimal" bind:value={amountStr} required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground" />
  </div>

  {#if !reinvested}
    <div>
      <label for="dividend-correction-payer" class="mb-1 block text-sm font-medium text-foreground">{m.income_coverage_payer()}</label>
      <select id="dividend-correction-payer" bind:value={payerID}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.income_coverage_payer_unassigned()}</option>
        {#each instrumentsQuery.data?.instruments ?? [] as instrument (instrument.commodity_id)}
          <option value={String(instrument.commodity_id)}>{instrument.display_name} ({instrument.symbol || instrument.commodity_code})</option>
        {/each}
      </select>
      <p class="mt-1 text-xs text-muted">{m.investments_dividend_payer_help()}</p>
    </div>
    <div>
      <label for="dividend-correction-ex-date" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_dividend_ex_date()}
        <span class="ml-1 text-xs font-normal text-muted">({m.investments_form_optional()})</span>
      </label>
      <input id="dividend-correction-ex-date" type="date" bind:value={exDate}
        aria-describedby="dividend-correction-ex-date-hint"
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
      <p id="dividend-correction-ex-date-hint" class="mt-1 text-xs text-muted">{m.investments_dividend_ex_date_hint()}</p>
    </div>
  {/if}

  {#if !withholdingOnly}
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

  {:else}
    <div>
      <label for="dividend-correction-tax-account" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_withholding_account()}</label>
      <select id="dividend-correction-tax-account" bind:value={withholdingAccountID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {#each withholdingAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
      </select>
    </div>
  {/if}
  {#if !reinvested && !withholdingOnly}
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
      class="min-h-10 rounded-(--radius-control) bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground transition-colors hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-50">
      {m.transactions_investment_replace_submit()}
    </button>
  </div>
</form>
