<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import { parseMoneyMagnitude } from '#lib/investments/form-amounts.ts';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import {
    gainAcknowledgement,
    gainImpactCurrency,
    gainImpactRows,
    hasGainChanges,
    impactNeedsReview,
    isGainAcknowledgementRefusal
  } from '#lib/investments/gain-impact.ts';
  import {
    previewTransferBasisResolution,
    resolveTransferBasis,
    type GainImpact,
    type InvestmentBasisResolutionRequest,
    type InvestmentCorrectionTransferTerms,
    type ReconciliationImpactResponse
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';

  // Resolves an external transfer in whose basis was recorded unknown (T-145).
  // The basis is the transfer's total sourced basis in its own currency; the
  // resolution posts the omitted cost-basis entry on the transfer date and
  // revises every sale and transfer the shares reached.
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
  const queryClient = useQueryClient();
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let basisStr = $state('');
  let reference = $state('');
  let reason = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let basisInputElement: HTMLInputElement | undefined = $state();

  $effect(() => {
    basisInputElement?.focus();
  });

  let review = $state<{
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    gainRefreshed: boolean;
    payload: InvestmentBasisResolutionRequest;
  } | null>(null);

  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency: CurrencyResponse) => [currency.id, currency])));
  const basisCurrency = $derived(currenciesByID.get(source.cost_commodity_id));
  const gainRows = $derived(review?.gainImpact
    ? gainImpactRows(review.gainImpact.changes, gainImpactCurrency(currenciesByID), getLocale()) : []);
  const canSubmit = $derived(!!basisStr.trim() && !!reason.trim());

  function buildPayload(): InvestmentBasisResolutionRequest | null {
    const basis = parseMoneyMagnitude(basisStr, { maxScale: 12 });
    if (!basis.ok) {
      formError = new TranslatedFormError(basis.reason === 'too_large'
        ? m.investments_form_amount_too_large() : m.investments_transfer_basis_error());
      return null;
    }
    if (basis.field.value === '0') {
      formError = new TranslatedFormError(m.investments_basis_resolution_zero_error());
      return null;
    }
    return {
      basis_value: basis.field.value, basis_scale: basis.field.scale, reason: reason.trim(),
      ...(reference.trim() ? { source_evidence: { reference: reference.trim() } } : {})
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
      if (!(await reviewImpact(payload, false))) await commit(payload, false, '');
    } catch (error) {
      await recover(error, payload);
    } finally {
      pending = false;
    }
  }

  // Preview through the actual resolution writer; checkpoints or the gains it
  // resolves go to the confirmation.
  async function reviewImpact(payload: InvestmentBasisResolutionRequest, refreshed: boolean): Promise<boolean> {
    const impact = await previewTransferBasisResolution(transactionID, payload);
    if (!impactNeedsReview(impact)) return false;
    const gainImpact = hasGainChanges(impact.gain_impact) ? impact.gain_impact : null;
    review = { impacts: impact.affected_checkpoints, gainImpact, gainRefreshed: refreshed && !!gainImpact, payload };
    return true;
  }

  async function commit(payload: InvestmentBasisResolutionRequest, override: boolean, acknowledgement: string) {
    await resolveTransferBasis(transactionID, {
      ...payload,
      ...(override ? { reconciliation_override: true } : {}),
      ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {})
    }, csrfToken);
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  async function recover(error: unknown, payload: InvestmentBasisResolutionRequest) {
    try {
      if (!isGainAcknowledgementRefusal(error) || !(await reviewImpact(payload, true))) formError = error;
    } catch (previewError) {
      formError = previewError;
    }
  }

  async function confirmReview() {
    if (!review) return;
    const { payload, impacts, gainImpact } = review;
    review = null;
    pending = true;
    formError = undefined;
    try {
      await commit(payload, impacts.length > 0, gainAcknowledgement(gainImpact));
    } catch (error) {
      await recover(error, payload);
    } finally {
      pending = false;
    }
  }
</script>

{#if review}
  <ReconciliationConfirm impacts={review.impacts} {gainRows} gainRefreshed={review.gainRefreshed}
    {pending} onCancel={() => (review = null)} onConfirm={confirmReview} />
{/if}

<form onsubmit={handleSubmit} class="space-y-4" aria-busy={pending}>
  <h2 class="text-base font-semibold text-foreground">{m.investments_basis_resolution_title()}</h2>
  <p class="text-sm text-muted">{m.investments_basis_resolution_copy()}</p>

  <div>
    <label for="basis-resolution-amount" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_basis_resolution_amount({ currency: basisCurrency?.code ?? '' })}
    </label>
    <input id="basis-resolution-amount" type="text" inputmode="decimal" bind:this={basisInputElement}
      bind:value={basisStr} required aria-describedby="basis-resolution-amount-help"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground" />
    <p id="basis-resolution-amount-help" class="mt-1 text-xs text-muted">{m.investments_basis_resolution_amount_help()}</p>
  </div>

  <div>
    <label for="basis-resolution-reference" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_transfer_source_reference()}
    </label>
    <input id="basis-resolution-reference" type="text" bind:value={reference} maxlength="200"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <div>
    <label for="basis-resolution-reason" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_basis_resolution_reason()}
    </label>
    <input id="basis-resolution-reason" type="text" bind:value={reason} maxlength="500" required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <APIFormError error={formError} id="basis-resolution-error" />

  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel} disabled={pending}
      class="min-h-10 rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    <button type="submit" disabled={!canSubmit || pending}
      class="min-h-10 rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
      {pending ? m.investments_basis_resolution_pending() : m.investments_basis_resolution_submit()}
    </button>
  </div>
</form>
