<script lang="ts">
  import { useEntryActivity } from '#lib/transactions/entry-context.svelte.ts';
  import { isInvestmentHoldingAccount } from '#lib/investments/settlement-accounts.ts';
  import { investmentEntryAccountsQueryOptions } from '#lib/api/investment-entry.ts';
  import { autofocus } from '#lib/components/autofocus.ts';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { untrack } from 'svelte';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryOptions, datedHoldingsQueryOptions, previewInvestmentSplit,
    previewSplitReplacement, previewSplitOperationReplacement, recordInvestmentSplit, replaceSplit, replaceSplitOperation, type GainImpact,
    type InvestmentCorrectionSplitTerms, type InvestmentSplitPlan, type InvestmentSplitReplacementRequest,
    type InvestmentSplitRequest, type ReconciliationImpactResponse
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import {
    gainImpactCurrency,
    gainImpactRows,
    hasGainChanges
  } from './gain-impact';
  import { previewReview, type ReviewGuards } from './preview-review';
  import ReconciliationConfirm from './reconciliation-confirm.svelte';
  import { evidenceText, newSourceEvidence, sourceEvidenceEdits } from './source-evidence';
  import { parseSplitRatio } from './split-ratio';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import { coefficientSign } from '#lib/money/amount.ts';
  import { formatQuantity } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { usePrefilledPosition } from '#lib/investments/investment-prefill.svelte.ts';
  import { restoredPositionKey } from '#lib/investments/investment-prefill.ts';
  import type { InvestmentPrefill } from '#lib/investments/investment-prefill.ts';

  const ownerToday = useOwnerToday();

  // With correction set, the form replaces that posted split (T-129): the
  // holding is fixed, the current terms are pre-filled and a reason is required.
  let { csrfToken, onSaved, onCancel, correction, prefill }: {
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
    prefill?: InvestmentPrefill;
    correction?: { transactionID?: number; operationID?: number; terms: InvestmentCorrectionSplitTerms };
  } = $props();
  const entryPrefill = untrack(() => correction ? undefined : prefill);

  // A correction carries its evidence edits beside the creation terms.
  type SplitPayload = InvestmentSplitRequest & Pick<InvestmentSplitReplacementRequest, 'source_evidence_edits'>;

  // Pre-select the holding the screen names (#186); a correction keeps its own.
  usePrefilledPosition(untrack(() => correction ? undefined : prefill),
    () => holdings.map((holding) => ({ account_id: holding.accountID, commodity_id: holding.commodityID, key: holding.key })),
    (holding) => holding.key, () => holdingKey, (key) => { holdingKey = key; },
    restoredPositionKey(untrack(() => correction ? undefined : prefill), false));

  const queryClient = useQueryClient();

  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  // Pre-fill once from the split being corrected; later edits are the user's.
  const initialTerms = untrack(() => correction?.terms);
  let effectiveOn = $state(initialTerms?.effective_on ?? entryPrefill?.transactionDate ?? '');
  let holdingKey = $state(initialTerms ? `${initialTerms.holding_account_id}:${initialTerms.commodity_id}` : '');
  let newUnits = $state(initialTerms ? String(initialTerms.ratio_numerator) : '');
  let oldUnits = $state(initialTerms ? String(initialTerms.ratio_denominator) : '');
  let reason = $state('');
  // The recorded evidence pre-fills the reference it was entered with; a
  // correction sends only a changed reference, so omitting it keeps the
  // recorded evidence exactly (#312).
  const initialEvidence = initialTerms?.source_evidence as Record<string, unknown> | undefined;
  let sourceReference = $state(evidenceText(initialEvidence, 'reference'));
  let memo = $state(entryPrefill?.memo ?? '');
  let pending = $state(false);
  const accountsQuery = createQuery(() => investmentEntryAccountsQueryOptions(effectiveOn || ownerToday()));
  let formError = $state<unknown>(undefined);
  // New transaction's working copy and shared context (#508).
  const workingCopy = useEntryActivity('split', {
    correction: !!initialTerms,
    values: () => ({ newUnits, oldUnits }),
    date: () => effectiveOn,
    memo: () => memo,
    reference: () => sourceReference,
    holding: () => selectedHolding?.accountID,
    instrument: () => selectedHolding?.commodityID,
    pending: () => pending
  });
  if (workingCopy) untrack(() => {
    newUnits = workingCopy.newUnits ?? newUnits;
    oldUnits = workingCopy.oldUnits ?? oldUnits;
  });
  untrack(() => { if (!initialTerms && prefill?.reference) sourceReference = prefill.reference; });
  // The reviewed preview. Editing any field discards it, so the user always
  // records exactly the plan they saw.
  let preview = $state<{
    plan: InvestmentSplitPlan;
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    payload: SplitPayload;
  } | null>(null);
  let reviewModal = $state<{ gainRefreshed: boolean } | null>(null);
  // Holdings as they stood at the split's date, including ones a later sale
  // closed (#166); before a date is chosen, today's.
  const holdingsDate = $derived(effectiveOn || ownerToday());
  const positionsQuery = createQuery(() => datedHoldingsQueryOptions(holdingsDate));

  const locale = $derived(getLocale());
  const accounts = $derived((accountsQuery.data?.accounts ?? []).filter((account) =>
    isInvestmentHoldingAccount(account)));
  // A split applies to the whole holding of a security in one account,
  // across every basis currency its lots carry.
  const holdings = $derived.by(() => {
    const seen = new Map<string, { accountID: number; commodityID: number }>();
    for (const position of positionsQuery.data ?? []) {
      // Splits adjust long lots only; a short-only holding has none (#173).
      if (position.position_side !== 'long' || coefficientSign(position.quantity_value) <= 0) continue;
      if (!accounts.some((account) => account.id === position.account_id)) continue;
      const key = `${position.account_id}:${position.commodity_id}`;
      if (!seen.has(key)) seen.set(key, { accountID: position.account_id, commodityID: position.commodity_id });
    }
    // An empty holding can still record a sourced split (#146). Include
    // its configured instrument even when it has no entitled long lots.
    for (const account of accounts) {
      const instrument = (instrumentsQuery.data?.instruments ?? []).find((item) =>
        item.commodity_id === account.default_commodity_id && item.status === 'active');
      if (!instrument) continue;
      const key = `${account.id}:${instrument.commodity_id}`;
      if (!seen.has(key)) seen.set(key, { accountID: account.id, commodityID: instrument.commodity_id });
    }
    return [...seen.entries()].map(([key, value]) => ({ key, ...value }));
  });
  // A corrected split's holding may since have been sold out; it stays fixed.
  const selectedHolding = $derived(correction
    ? { key: holdingKey, accountID: correction.terms.holding_account_id, commodityID: correction.terms.commodity_id }
    : holdings.find((holding) => holding.key === holdingKey));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency) => [currency.id, currency])));
  const instrumentLabel = (commodityID: number) => {
    const instrument = (instrumentsQuery.data?.instruments ?? []).find((item) => item.commodity_id === commodityID);
    return instrument?.display_name ?? instrument?.commodity_code ?? `#${commodityID}`;
  };
  const loading = $derived(accountsQuery.isPending || positionsQuery.isPending ||
    instrumentsQuery.isPending || currenciesQuery.isPending);
  const loadError = $derived(accountsQuery.isError || positionsQuery.isError ||
    instrumentsQuery.isError || currenciesQuery.isError);
  const canPreview = $derived(!loading && !loadError && !!effectiveOn && !!selectedHolding &&
    !!newUnits.trim() && !!oldUnits.trim() && (!correction || !!reason.trim()));
  const modalGainRows = $derived(preview?.gainImpact
    ? gainImpactRows(preview.gainImpact.changes, gainImpactCurrency(currenciesByID), locale)
    : []);


  function discardPreview() {
    preview = null;
    formError = undefined;
  }

  function replacementRequest(payload: SplitPayload): InvestmentSplitReplacementRequest {
    return {
      reason: reason.trim(), effective_on: payload.effective_on,
      ratio_numerator: payload.ratio_numerator, ratio_denominator: payload.ratio_denominator,
      source_evidence_edits: payload.source_evidence_edits, memo: payload.memo
    };
  }

  async function runPreview(payload: SplitPayload): Promise<void> {
    const result = correction
      ? await (correction.operationID ? previewSplitOperationReplacement(correction.operationID, replacementRequest(payload)) : previewSplitReplacement(correction.transactionID!, replacementRequest(payload)))
      : await previewInvestmentSplit(payload);
    preview = {
      plan: result.plan,
      impacts: result.impact.affected_checkpoints,
      gainImpact: hasGainChanges(result.impact.gain_impact) ? result.impact.gain_impact : null,
      payload
    };
  }

  async function handlePreview(event: SubmitEvent) {
    event.preventDefault();
    if (!canPreview || !selectedHolding) return;
    const ratio = parseSplitRatio(newUnits, oldUnits);
    if (!ratio.ok) {
      formError = new TranslatedFormError(m.investments_split_ratio_error());
      return;
    }
    pending = true;
    formError = undefined;
    try {
      await runPreview({
        effective_on: effectiveOn,
        holding_account_id: selectedHolding.accountID,
        commodity_id: selectedHolding.commodityID,
        ratio_numerator: ratio.numerator,
        ratio_denominator: ratio.denominator,
        ...(correction
          ? { source_evidence_edits: sourceEvidenceEdits(initialEvidence, { reference: sourceReference }) }
          : { source_evidence: newSourceEvidence({ reference: sourceReference }) }),
        memo: memo.trim() || undefined
      });
    } catch (error) {
      preview = null;
      formError = error;
    } finally {
      pending = false;
    }
  }

  async function record(current: NonNullable<typeof preview>, confirmations: ReviewGuards) {
    if (correction) {
      await (correction.operationID ? replaceSplitOperation(correction.operationID, { ...replacementRequest(current.payload), ...confirmations }, csrfToken) : replaceSplit(correction.transactionID!, { ...replacementRequest(current.payload), ...confirmations }, csrfToken));
    } else {
      await recordInvestmentSplit({ ...current.payload, ...confirmations }, csrfToken);
    }
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  // The server recomputes everything at commit. A changed gain or checkpoint
  // set re-opens the review with the current figures instead of a dead-end
  // error, and the write is never retried on its own (#400).
  const review = previewReview<SplitPayload, NonNullable<typeof preview>>({
    get preview() { return preview; },
    refresh: runPreview,
    write: record,
    discardPreview: () => { preview = null; },
    setPending: (value) => { pending = value; },
    setError: (error) => { formError = error; },
    setModal: (modal) => { reviewModal = modal; }
  });
</script>

{#if reviewModal && preview}
  <ReconciliationConfirm impacts={preview.impacts} gainRows={modalGainRows}
    gainRefreshed={reviewModal.gainRefreshed} {pending}
    onCancel={review.cancel} onConfirm={review.confirm} />
{/if}

<form onsubmit={handlePreview} class="space-y-4" aria-busy={pending}>
  <!-- Nothing in the form changes while its save is in flight (#508). -->
  <fieldset disabled={pending} class="min-w-0 space-y-4">
  <h2 id="split-title" class="text-base font-semibold text-foreground">
    {correction ? m.transactions_investment_replace_split_title() : m.investments_split_title()}
  </h2>
  <p class="text-sm text-muted">{correction ? m.investments_split_replace_help() : m.investments_split_help()}</p>
  {#if loading}
    <p class="text-sm text-muted" role="status">{m.investments_loading()}</p>
  {:else if loadError}
    <p class="text-sm text-danger" role="alert">{m.investments_split_load_error()}</p>
  {:else}
    {#if holdings.length === 0 && !correction}
      <p class="text-sm text-muted" role="status">{m.investments_split_empty()}</p>
    {/if}
    <div>
      <label for="split-holding" class="mb-1 block text-sm font-medium text-foreground">{m.investments_split_holding()}</label>
      {#if correction}
        <p id="split-holding" class="rounded-(--radius-control) border border-border bg-surface px-3 py-2 text-sm text-foreground">
          {accounts.find((account) => account.id === correction.terms.holding_account_id)?.name ?? `#${correction.terms.holding_account_id}`}
          · {instrumentLabel(correction.terms.commodity_id)}
        </p>
      {:else}
      <select id="split-holding" aria-describedby="split-holding-hint" bind:value={holdingKey} required onchange={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_split_select_holding()}</option>
        {#if holdingKey && !holdings.some((item) => item.key === holdingKey)}<option value={holdingKey} disabled>{m.investments_activity_position_unavailable_option()}</option>{/if}
        {#each holdings as holding (holding.key)}
          <option value={holding.key}>
            {accounts.find((account) => account.id === holding.accountID)?.name} · {instrumentLabel(holding.commodityID)}
          </option>
        {/each}
      </select>
      {#if positionsQuery.data && holdingKey && !holdings.some((item) => item.key === holdingKey)}<p role="alert" class="mt-1 text-sm text-danger">{m.investments_activity_position_unavailable()}</p>{/if}
      <p id="split-holding-hint" class="mt-1 text-xs text-muted">{m.investments_split_holding_hint()}</p>
      {/if}
    </div>
    <div>
      <label for="split-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_split_effective_date()}</label>
      <input id="split-date" type="date" {@attach autofocus} bind:value={effectiveOn} required oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <fieldset class="space-y-2">
      <legend class="text-sm font-medium text-foreground">{m.investments_split_ratio()}</legend>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="split-new-units" class="mb-1 block text-xs text-muted">{m.investments_split_ratio_new()}</label>
          <input id="split-new-units" type="text" inputmode="numeric" autocomplete="off" bind:value={newUnits}
            required oninput={discardPreview} aria-describedby="split-ratio-help"
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
        </div>
        <div>
          <label for="split-old-units" class="mb-1 block text-xs text-muted">{m.investments_split_ratio_old()}</label>
          <input id="split-old-units" type="text" inputmode="numeric" autocomplete="off" bind:value={oldUnits}
            required oninput={discardPreview} aria-describedby="split-ratio-help"
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
        </div>
      </div>
      <p id="split-ratio-help" class="text-xs text-muted">{m.investments_split_ratio_help()}</p>
    </fieldset>
    <div>
      <label for="split-reference" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_source_reference()}</label>
      <input id="split-reference" type="text" bind:value={sourceReference} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    {#if correction}
      <div>
        <label for="split-reason" class="mb-1 block text-sm font-medium text-foreground">{m.investments_split_replace_reason()}</label>
        <input id="split-reason" type="text" bind:value={reason} maxlength="500" required oninput={discardPreview}
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
      </div>
    {/if}
    <div>
      <label for="split-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
      <input id="split-memo" type="text" bind:value={memo} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    {#if preview}
      <section class="space-y-2 rounded-(--radius-control) border border-border p-3" aria-labelledby="split-preview-title" aria-live="polite">
        <h3 id="split-preview-title" class="text-sm font-semibold text-foreground">{m.investments_split_preview_title()}</h3>
        <ul class="space-y-1 text-sm text-foreground">
          {#each preview.plan.effects as effect (effect.lot_id)}
            <li class="font-mono">{m.investments_split_lot_change({
              lot: String(effect.lot_id),
              before: formatQuantity(effect.quantity_before_value, effect.quantity_before_scale, locale),
              after: formatQuantity(effect.quantity_after_value, effect.quantity_after_scale, locale)
            })}</li>
          {/each}
        </ul>
        <p class="text-sm font-medium text-foreground">{m.investments_split_total_change({
          delta: formatQuantity(preview.plan.quantity_delta_value, preview.plan.quantity_delta_scale, locale)
        })}</p>
        <p class="text-xs text-muted">{m.investments_split_basis_kept()}</p>
        {#if preview.plan.replayed}
          <p class="text-xs text-warning" role="note">{m.investments_split_replayed()}</p>
        {/if}
      </section>
    {/if}
  {/if}
  <APIFormError error={formError} id="split-form-error" />
  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel}
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    {#if preview}
      <button type="button" onclick={review.record} disabled={pending || !csrfToken}
        class="rounded-(--radius-control) bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_split_pending() : correction ? m.investments_split_replace_submit() : m.investments_split_submit()}
      </button>
    {:else}
      <button type="submit" disabled={!canPreview || pending}
        class="rounded-(--radius-control) bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_split_previewing() : m.investments_split_preview()}
      </button>
    {/if}
  </div>
  </fieldset>
</form>
