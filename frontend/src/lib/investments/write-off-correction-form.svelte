<script lang="ts">
  import { autofocus } from '#lib/components/autofocus.ts';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { accountsQueryOptions, type AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import { formatEditableAmount } from '#lib/money/amount.ts';
  import { costBasisMethodLabel, formatScaledValue } from '#lib/investments/investment-labels.ts';
  import { parseMagnitude, type AmountFieldError } from '#lib/investments/form-amounts.ts';
  import { correctionLotDrafts, parseCorrectionLotChoices, type CorrectionLotDraft } from '#lib/investments/trade-economics.ts';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { gainImpactCurrency, gainImpactRows } from '#lib/investments/gain-impact.ts';
  import { commitReview, type CommitReviewState } from '#lib/investments/commit-review.ts';
  import type { ReviewGuards } from '#lib/investments/preview-review.ts';
  import {
    previewWriteOffReplacementReconciliation,
    replaceWriteOff,
    type InvestmentTradeCorrectionContextResponse,
    type InvestmentWriteOffRequest
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';

  // Replaces a posted write-off (T-118). Proceeds stay zero and there is no
  // cash leg; quantity, method, lot elections, date and holding may change.
  // The instrument is shown read-only.
  let {
    csrfToken,
    correction,
    onSaved,
    onCancel
  }: {
    csrfToken: string;
    correction: InvestmentTradeCorrectionContextResponse;
    onSaved: () => void;
    onCancel: () => void;
  } = $props();

  type Method = 'fifo' | 'lifo' | 'average_cost' | 'specific_lot';
  const source = untrack(() => correction);
  const locale = getLocale();
  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let reason = $state('');
  let transactionDate = $state(source.event_date);
  let holdingAccountID = $state(String(source.holding_account_id));
  let quantityStr = $state(formatEditableAmount(source.quantity_value, source.quantity_scale));
  let method = $state<Method>((source.cost_basis_method as Method) || 'fifo');
  let lotChoices = $state<CorrectionLotDraft[]>(correctionLotDrafts(source));
  let writeOffReason = $state(source.memo ?? '');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);


  let review = $state<CommitReviewState<InvestmentWriteOffRequest> | null>(null);

  const holdingAccounts = $derived((accountsQuery.data?.accounts ?? []).filter((account: AccountResponse) =>
    (account.account_kind === 'security_holding' || account.account_kind === 'fund_holding') &&
    (account.status === 'active' || account.id === source.holding_account_id)));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency: CurrencyResponse) => [currency.id, currency])));
  const gainRows = $derived(review?.gainImpact
    ? gainImpactRows(review.gainImpact.changes, gainImpactCurrency(currenciesByID), locale) : []);
  // The listed lots belong to the recorded holding; a moved write-off uses an
  // order method instead.
  const moved = $derived(Number(holdingAccountID) !== source.holding_account_id);
  const methods = $derived<Method[]>(moved
    ? ['fifo', 'lifo', 'average_cost'] : ['fifo', 'lifo', 'average_cost', 'specific_lot']);
  $effect(() => {
    if (moved && method === 'specific_lot') method = 'fifo';
  });
  const canSubmit = $derived(!!reason.trim() && !!writeOffReason.trim() && transactionDate !== '' &&
    holdingAccountID !== '' && quantityStr.trim() !== '');

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
    const quantity = parseMagnitude(quantityStr);
    if (!quantity.ok) {
      formError = new Error(amountErrorMessage(quantity.reason));
      return;
    }
    const lots = method === 'specific_lot'
      ? parseCorrectionLotChoices(lotChoices, source.available_lots, quantity.field) : null;
    if (lots && !lots.ok) {
      formError = new Error(lots.reason === 'exceeds_available'
        ? m.transactions_investment_replace_lot_exceeds()
        : lots.reason === 'mismatch'
          ? m.transactions_investment_replace_lot_mismatch()
          : m.transactions_investment_replace_lot_invalid());
      return;
    }
    const payload: InvestmentWriteOffRequest = {
      transaction_date: transactionDate, commodity_id: source.commodity_id,
      holding_account_id: Number(holdingAccountID),
      cost_commodity_id: source.cost_commodity_id,
      quantity_value: quantity.field.value, quantity_scale: quantity.field.scale,
      reason: writeOffReason.trim(), cost_basis_method: method,
      lot_allocations: lots?.ok ? lots.allocations : undefined, payee_id: source.payee_id
    };
    pending = true;
    formError = undefined;
    try {
      if (!(await impactReview.open(payload, false))) await commit(payload, {});
    } catch (error) {
      await impactReview.recover(error, payload);
    } finally {
      pending = false;
    }
  }

  async function commit(payload: InvestmentWriteOffRequest, guards: ReviewGuards) {
    await replaceWriteOff(source.transaction_id, { reason: reason.trim(), replacement: payload, ...guards }, csrfToken);
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  // The gain set changed since review: show the current set, not a dead end.
  const impactReview = commitReview({
    protocol: 'gains',
    get review() { return review; },
    impact: (payload: InvestmentWriteOffRequest) => previewWriteOffReplacementReconciliation(source.transaction_id,
      { reason: reason.trim(), replacement: payload }),
    write: commit,
    setReview: (value) => { review = value; },
    setPending: (value) => { pending = value; },
    setError: (error) => { formError = error; }
  });
</script>

{#if review}
  <ReconciliationConfirm impacts={review.impacts} {gainRows} gainRefreshed={review.gainRefreshed}
    {pending} onCancel={impactReview.cancel} onConfirm={impactReview.confirm} />
{/if}

<form onsubmit={handleSubmit} aria-busy={pending} class="space-y-4">
  <h2 class="text-base font-semibold text-foreground">{m.transactions_investment_replace_write_off_title()}</h2>
  <p class="text-sm text-muted">{m.transactions_investment_replace_write_off_copy()}</p>

  <div>
    <label for="write-off-correction-reason" class="mb-1 block text-sm font-medium text-foreground">
      {m.transactions_investment_replace_reason()}
    </label>
    <input id="write-off-correction-reason" type="text" {@attach autofocus} bind:value={reason}
      maxlength="500" required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <p class="text-sm text-foreground">{m.investments_form_instrument()}: <strong>{source.commodity_code}</strong></p>
  <p class="text-sm text-foreground">{m.investments_transfer_basis_currency()}: <strong>{currenciesByID.get(source.cost_commodity_id)?.code ?? `#${source.cost_commodity_id}`}</strong></p>

  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
    <div>
      <label for="write-off-correction-date" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_form_date()}
      </label>
      <input id="write-off-correction-date" type="date" bind:value={transactionDate} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="write-off-correction-holding" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_form_holding_account()}
      </label>
      <select id="write-off-correction-holding" bind:value={holdingAccountID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {#each holdingAccounts as account (account.id)}
          <option value={String(account.id)}>{account.name}</option>
        {/each}
      </select>
    </div>
  </div>

  <div>
    <label for="write-off-correction-quantity" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_quantity()}
    </label>
    <input id="write-off-correction-quantity" type="text" inputmode="decimal" bind:value={quantityStr} required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground" />
  </div>

  <div>
    <label for="write-off-correction-cause" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_write_off_reason()}
    </label>
    <input id="write-off-correction-cause" type="text" bind:value={writeOffReason} maxlength="500" required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <div>
    <label for="write-off-correction-method" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_form_cost_basis_method()}
    </label>
    <select id="write-off-correction-method" bind:value={method}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
      {#each methods as value (value)}
        <option {value}>{costBasisMethodLabel(value)}</option>
      {/each}
    </select>
    {#if moved}
      <p class="mt-1 text-xs text-muted">{m.transactions_investment_replace_moved_lots_hint()}</p>
    {/if}
  </div>

  {#if method === 'specific_lot'}
    <fieldset class="space-y-3 rounded-(--radius-control) border border-border p-3">
      <legend class="px-1 text-sm font-medium text-foreground">{m.transactions_investment_replace_lots_title()}</legend>
      <p class="text-xs text-muted">{m.transactions_investment_replace_lots_copy()}</p>
      {#each lotChoices as choice, index (index)}
        <div class="grid grid-cols-1 gap-2 rounded-(--radius-control) border border-border p-2 sm:grid-cols-[minmax(0,1fr)_8rem_auto]">
          <label class="text-xs text-muted">{m.transactions_investment_replace_lot()}
            <select bind:value={choice.lotID} required
              class="mt-1 w-full rounded-(--radius-control) border border-border bg-control px-2 py-2 text-sm text-foreground">
              <option value="">{m.transactions_investment_replace_lot_select()}</option>
              {#each source.available_lots as lot (lot.lot_id)}
                <option value={String(lot.lot_id)}>
                  #{lot.lot_id} · {lot.opened_on} · {formatScaledValue(lot.quantity_value, lot.quantity_scale, locale)}
                </option>
              {/each}
            </select>
          </label>
          <label class="text-xs text-muted">{m.investments_form_quantity()}
            <input type="text" inputmode="decimal" bind:value={choice.quantity} required
              class="mt-1 w-full rounded-(--radius-control) border border-border bg-control px-2 py-2 text-sm font-mono text-foreground" />
          </label>
          <button type="button" onclick={() => (lotChoices = lotChoices.filter((_, current) => current !== index))}
            class="self-end rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
            {m.transactions_investment_replace_lot_remove()}
          </button>
        </div>
      {/each}
      <button type="button" onclick={() => (lotChoices = [...lotChoices, { lotID: '', quantity: '' }])}
        class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {m.transactions_investment_replace_lot_add()}
      </button>
    </fieldset>
  {/if}

  <APIFormError error={formError} id="write-off-correction-error" />

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
