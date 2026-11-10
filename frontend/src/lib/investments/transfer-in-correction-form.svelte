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
  import { parseTransferInAmounts } from '#lib/investments/form-amounts.ts';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import { gainImpactCurrency, gainImpactRows } from '#lib/investments/gain-impact.ts';
  import { commitReview, type CommitReviewState } from '#lib/investments/commit-review.ts';
  import type { ReviewGuards } from '#lib/investments/preview-review.ts';
  import {
    previewTransferInReplacement,
    replaceTransferIn,
    type ExternalTransferInRequest,
    type InvestmentCorrectionTransferTerms
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';

  // Replaces a posted external transfer in (T-119). The security and basis
  // currency are fixed; the date, holding, quantity, carried basis and
  // original acquisition date may change. The inverse and the corrected
  // transfer both post, so the equity bridge difference is appended.
  let {
    csrfToken,
    transactionID,
    transfer,
    onSaved,
    onCancel
  }: {
    csrfToken: string;
    transactionID: number;
    transfer: InvestmentCorrectionTransferTerms;
    onSaved: () => void;
    onCancel: () => void;
  } = $props();

  const source = untrack(() => transfer);
  const locale = getLocale();
  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let reason = $state('');
  let effectiveOn = $state(source.effective_on);
  let holdingAccountID = $state(String(source.destination_account_id));
  let quantityStr = $state(source.quantity_value !== null && source.quantity_scale !== null
    ? formatEditableAmount(source.quantity_value, source.quantity_scale) : '');
  let carriedBasisStr = $state(source.carried_basis_value !== null && source.carried_basis_scale !== null
    ? formatEditableAmount(source.carried_basis_value, source.carried_basis_scale) : '');
  // Prefilled from the committed link; unknown is never shown as a zero.
  let basisUnknown = $state(source.basis_knowledge === 'unknown');
  let originalAcquiredOn = $state(source.original_acquired_on ?? '');
  let memo = $state(source.memo);
  let pending = $state(false);
  let formError = $state<unknown>(undefined);


  let review = $state<CommitReviewState<ExternalTransferInRequest> | null>(null);

  const holdingAccounts = $derived((accountsQuery.data?.accounts ?? []).filter((account: AccountResponse) =>
    (account.account_kind === 'security_holding' || account.account_kind === 'fund_holding') &&
    (account.status === 'active' || account.id === source.destination_account_id)));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency: CurrencyResponse) => [currency.id, currency])));
  const basisCurrency = $derived(currenciesByID.get(source.cost_commodity_id));
  const gainRows = $derived(review?.gainImpact
    ? gainImpactRows(review.gainImpact.changes, gainImpactCurrency(currenciesByID), locale) : []);
  const canSubmit = $derived(!!reason.trim() && effectiveOn !== '' && holdingAccountID !== '' &&
    !!quantityStr.trim() && (basisUnknown || !!carriedBasisStr.trim()));

  function buildPayload(): ExternalTransferInRequest | null {
    const amounts = parseTransferInAmounts({ quantityStr, carriedBasisStr: basisUnknown ? '' : carriedBasisStr, basisUnknown });
    if (!amounts.ok) {
      formError = new TranslatedFormError(amounts.reason === 'too_large'
        ? m.investments_form_amount_too_large()
        : amounts.field === 'quantity'
          ? m.investments_transfer_quantity_error()
          : m.investments_transfer_basis_error());
      return null;
    }
    if (originalAcquiredOn && originalAcquiredOn > effectiveOn) {
      formError = new TranslatedFormError(m.investments_transfer_original_date_error());
      return null;
    }
    return {
      effective_on: effectiveOn, holding_account_id: Number(holdingAccountID), commodity_id: source.commodity_id,
      quantity_value: amounts.values.quantity.value, quantity_scale: amounts.values.quantity.scale,
      ...(amounts.values.carriedBasis
        ? { carried_basis_value: amounts.values.carriedBasis.value, carried_basis_scale: amounts.values.carriedBasis.scale }
        : { basis_knowledge: 'unknown' as const }),
      cost_commodity_id: source.cost_commodity_id, original_acquired_on: originalAcquiredOn || undefined,
      // No source_evidence: this form does not edit it, and sending back the
      // parsed object would round its numbers. The server keeps the recorded
      // evidence exactly (#312).
      memo: memo.trim() || undefined
    };
  }

  async function handleSubmit(event: Event) {
    event.preventDefault();
    if (!canSubmit || pending) return;
    formError = undefined;
    const payload = buildPayload();
    if (!payload) return;
    pending = true;
    try {
      if (!(await impactReview.open(payload, false))) await commit(payload, {});
    } catch (error) {
      await impactReview.recover(error, payload);
    } finally {
      pending = false;
    }
  }

  async function commit(payload: ExternalTransferInRequest, guards: ReviewGuards) {
    await replaceTransferIn(transactionID, { reason: reason.trim(), replacement: payload, ...guards }, csrfToken);
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  // The gain set changed since review: show the current set, not a dead end.
  const impactReview = commitReview({
    protocol: 'gains',
    get review() { return review; },
    impact: (payload: ExternalTransferInRequest) => previewTransferInReplacement(transactionID, { reason: reason.trim(), replacement: payload }),
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

<form onsubmit={handleSubmit} class="space-y-4" aria-busy={pending}>
  <h2 class="text-base font-semibold text-foreground">{m.transactions_investment_replace_transfer_title()}</h2>
  <p class="text-sm text-muted">{m.transactions_investment_replace_transfer_in_copy()}</p>

  <div>
    <label for="transfer-in-correction-reason" class="mb-1 block text-sm font-medium text-foreground">
      {m.transactions_investment_replace_reason()}
    </label>
    <input id="transfer-in-correction-reason" type="text" {@attach autofocus} bind:value={reason}
      maxlength="500" required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
    <div>
      <label for="transfer-in-correction-date" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_transfer_effective_date()}
      </label>
      <input id="transfer-in-correction-date" type="date" bind:value={effectiveOn} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="transfer-in-correction-holding" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_form_holding_account()}
      </label>
      <select id="transfer-in-correction-holding" bind:value={holdingAccountID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {#each holdingAccounts as account (account.id)}
          <option value={String(account.id)}>{account.name}</option>
        {/each}
      </select>
    </div>
  </div>

  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
    <div>
      <label for="transfer-in-correction-quantity" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_form_quantity()}
      </label>
      <input id="transfer-in-correction-quantity" type="text" inputmode="decimal" bind:value={quantityStr} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground" />
    </div>
    <div>
      <label for="transfer-in-correction-basis" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_transfer_carried_basis()} {basisCurrency?.code ?? ''}
      </label>
      <input id="transfer-in-correction-basis" type="text" inputmode="decimal" bind:value={carriedBasisStr} required={!basisUnknown}
        disabled={basisUnknown} aria-describedby="transfer-in-correction-basis-help"
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground disabled:opacity-60" />
      <label class="mt-2 flex items-start gap-2 text-sm text-foreground">
        <input type="checkbox" bind:checked={basisUnknown} aria-describedby="transfer-in-correction-basis-help" class="mt-0.5" />
        <span>{m.investments_transfer_basis_unknown()}</span>
      </label>
      <p id="transfer-in-correction-basis-help" class="mt-1 text-xs text-muted">
        {basisUnknown ? m.investments_transfer_basis_unknown_help() : m.investments_transfer_basis_help()}
      </p>
    </div>
  </div>

  <div>
    <label for="transfer-in-correction-original" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_transfer_original_date()}
    </label>
    <input id="transfer-in-correction-original" type="date" bind:value={originalAcquiredOn}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <div>
    <label for="transfer-in-correction-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
    <input id="transfer-in-correction-memo" type="text" bind:value={memo} maxlength="500"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <APIFormError error={formError} id="transfer-in-correction-error" />

  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel} disabled={pending}
      class="min-h-10 rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    <button type="submit" disabled={!canSubmit || pending}
      class="min-h-10 rounded-(--radius-control) bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
      {m.transactions_investment_replace_submit()}
    </button>
  </div>
</form>
