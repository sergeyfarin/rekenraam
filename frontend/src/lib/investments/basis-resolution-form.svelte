<script lang="ts">
  import { autofocus } from '#lib/components/autofocus.ts';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import { parseMoneyMagnitude } from '#lib/investments/form-amounts.ts';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import { gainImpactCurrency, gainImpactRows } from '#lib/investments/gain-impact.ts';
  import { commitReview, type CommitReviewState } from '#lib/investments/commit-review.ts';
  import type { ReviewGuards } from '#lib/investments/preview-review.ts';
  import { formatEditableAmount } from '#lib/money/amount.ts';
  import {
    previewBasisResolutionReplacement,
    previewTransferBasisResolution,
    replaceBasisResolution,
    resolveTransferBasis,
    type InvestmentCorrectionBasisResolutionTerms,
    type InvestmentBasisResolutionRequest
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import { basisSourceDeclaration, basisSourceFields, basisSourceTextMaxLength, basisSourceTextTooLong } from './basis-provenance';

  // Resolves an external transfer in whose basis was recorded unknown (T-145),
  // or, given the effective resolution, corrects it (#168). The basis is the
  // transfer's total sourced basis in its own currency; the resolution posts
  // the omitted cost-basis entry on the transfer date (none for a known zero)
  // and revises every sale and transfer the shares reached.
  let {
    csrfToken,
    transactionID,
    costCommodityID,
    current = null,
    onSaved,
    onCancel
  }: {
    csrfToken: string;
    transactionID: number;
    costCommodityID: number;
    current?: InvestmentCorrectionBasisResolutionTerms | null;
    onSaved: () => void;
    onCancel: () => void;
  } = $props();

  const correcting = untrack(() => current);
  const queryClient = useQueryClient();
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let basisStr = $state(correcting
    ? formatEditableAmount(correcting.basis_value, correcting.basis_scale) : '');
  const initialSource = basisSourceFields(correcting?.source_evidence);
  let sourceKind = $state(correcting ? initialSource.kind : 'statement');
  let reference = $state(initialSource.reference);
  let rationale = $state(initialSource.rationale);
  let knownOn = $state(initialSource.knownOn);
  let reason = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);


  let review = $state<CommitReviewState<InvestmentBasisResolutionRequest> | null>(null);

  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency: CurrencyResponse) => [currency.id, currency])));
  const basisCurrency = $derived(currenciesByID.get(costCommodityID));
  const gainRows = $derived(review?.gainImpact
    ? gainImpactRows(review.gainImpact.changes, gainImpactCurrency(currenciesByID), getLocale()) : []);
  const declaration = $derived(basisSourceDeclaration(sourceKind, reference, rationale, knownOn));
  const sourceTextTooLong = $derived(basisSourceTextTooLong(sourceKind === 'owner_assertion' ? rationale : reference));
  const canSubmit = $derived(!!basisStr.trim() && !!reason.trim() && declaration !== null);

  function buildPayload(): InvestmentBasisResolutionRequest | null {
    if (!declaration) return null;
    const basis = parseMoneyMagnitude(basisStr, { maxScale: 12 });
    if (!basis.ok) {
      formError = new TranslatedFormError(basis.reason === 'too_large'
        ? m.investments_form_amount_too_large() : m.investments_transfer_basis_error());
      return null;
    }
    return {
      basis_value: basis.field.value, basis_scale: basis.field.scale, reason: reason.trim(),
      source_evidence: declaration,
      ...(correcting ? {
        expected_resolution_operation_id: correcting.operation_id,
        expected_provenance_revision: correcting.provenance_revision,
        preserve_source_metadata: true
      } : {})
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

  async function commit(payload: InvestmentBasisResolutionRequest, guards: ReviewGuards) {
    const body = { ...payload, ...guards };
    if (correcting) await replaceBasisResolution(transactionID, body, csrfToken);
    else await resolveTransferBasis(transactionID, body, csrfToken);
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  // Preview through the actual resolution writer; checkpoints or the gains it
  // resolves go to the confirmation.
  const impactReview = commitReview({
    protocol: 'gains',
    get review() { return review; },
    impact: (payload: InvestmentBasisResolutionRequest) => correcting
      ? previewBasisResolutionReplacement(transactionID, payload)
      : previewTransferBasisResolution(transactionID, payload),
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
  <h2 class="text-base font-semibold text-foreground">
    {correcting ? m.investments_basis_resolution_correct_title() : m.investments_basis_resolution_title()}
  </h2>
  <p class="text-sm text-muted">
    {correcting ? m.investments_basis_resolution_correct_copy() : m.investments_basis_resolution_copy()}
  </p>

  <div>
    <label for="basis-resolution-amount" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_basis_resolution_amount({ currency: basisCurrency?.code ?? '' })}
    </label>
    <input id="basis-resolution-amount" type="text" inputmode="decimal" {@attach autofocus}
      bind:value={basisStr} required aria-describedby="basis-resolution-amount-help"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground" />
    <p id="basis-resolution-amount-help" class="mt-1 text-xs text-muted">{m.investments_basis_resolution_amount_help()}</p>
  </div>

  <div>
    <label for="basis-resolution-source-kind" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_basis_source_kind()}
    </label>
    <select id="basis-resolution-source-kind" bind:value={sourceKind} required aria-describedby="basis-source-help"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
      <option value="" disabled>{m.investments_basis_source_choose()}</option>
      <option value="statement">{m.investments_basis_source_statement()}</option>
      <option value="provider">{m.investments_basis_source_provider()}</option>
      <option value="owner_assertion">{m.investments_basis_source_owner()}</option>
    </select>
    <p id="basis-source-help" class="mt-1 text-xs text-muted">{m.investments_basis_source_help()}</p>
  </div>

  {#if sourceKind === 'owner_assertion'}
    <div>
      <label for="basis-resolution-rationale" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_basis_source_rationale()}
      </label>
      <textarea id="basis-resolution-rationale" bind:value={rationale} required rows="2"
        aria-invalid={sourceTextTooLong} aria-describedby={sourceTextTooLong ? "basis-resolution-error" : undefined}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"></textarea>
    </div>
    <div>
      <label for="basis-resolution-known-on" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_basis_source_known_on()}
      </label>
      <input id="basis-resolution-known-on" type="date" bind:value={knownOn} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
  {:else}
    <div>
      <label for="basis-resolution-reference" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_basis_source_reference()}
      </label>
      <input id="basis-resolution-reference" type="text" bind:value={reference} required
        aria-invalid={sourceTextTooLong} aria-describedby={sourceTextTooLong ? "basis-resolution-error" : undefined}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
  {/if}

  <div>
    <label for="basis-resolution-reason" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_basis_resolution_reason()}
    </label>
    <input id="basis-resolution-reason" type="text" bind:value={reason} maxlength="500" required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <APIFormError error={sourceTextTooLong
    ? new TranslatedFormError(m.investments_basis_source_too_long({ count: basisSourceTextMaxLength }))
    : formError} id="basis-resolution-error" />

  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel} disabled={pending}
      class="min-h-10 rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    <button type="submit" disabled={!canSubmit || pending}
      class="min-h-10 rounded-(--radius-control) bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground transition-colors hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-50">
      {pending ? m.investments_basis_resolution_pending()
        : correcting ? m.investments_basis_resolution_correct_submit() : m.investments_basis_resolution_submit()}
    </button>
  </div>
</form>
