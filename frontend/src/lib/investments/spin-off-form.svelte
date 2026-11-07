<script lang="ts">
  import { isInvestmentHoldingAccount } from '#lib/investments/settlement-accounts.ts';
  import { investmentEntryAccountsQueryOptions } from '#lib/api/investment-entry.ts';
  import { autofocus } from '#lib/components/autofocus.ts';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryOptions, datedHoldingsQueryOptions, previewSpinOff, recordSpinOff,
    previewSpinOffReplacement, replaceSpinOff,
    type GainImpact, type InvestmentCorrectionSpinOffTerms, type ReconciliationImpactResponse,
    type SpinOffPlan, type SpinOffReplacementRequest, type SpinOffRequest
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import {
    gainImpactCurrency,
    gainImpactRows,
    hasGainChanges
  } from './gain-impact';
  import { previewReview, type ReviewGuards } from './preview-review';
  import ReconciliationConfirm from './reconciliation-confirm.svelte';
  import SpinOffSummary from './spin-off-summary.svelte';
  import { evidenceText, newSourceEvidence, sourceEvidenceEdits } from './source-evidence';
  import { basisFractionPercent, parseBasisPercent, parseExchangeRatio } from './split-ratio';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import { coefficientSign, formatEditableAmount } from '#lib/money/amount.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { usePrefilledPosition } from '#lib/investments/investment-prefill.svelte.ts';
  import type { InvestmentPrefill } from '#lib/investments/investment-prefill.ts';

  const ownerToday = useOwnerToday();

  // Spin-off entry (#180): the parent holding keeps its units, a new
  // instrument is distributed to it, and the issuer's published share of the
  // basis moves to the new lots. The server plans every lot; the form previews
  // that plan and records exactly what was previewed. A spin-off dated behind
  // later activity revises it, which the shared gain review discloses (#183).
  // With correction set, the form replaces that posted spin-off: the parent
  // holding is fixed, the current terms are pre-filled and a reason is
  // required.
  let { csrfToken, onSaved, onCancel, correction, prefill }: {
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
    prefill?: InvestmentPrefill;
    correction?: { transactionID: number; terms: InvestmentCorrectionSpinOffTerms };
  } = $props();
  const entryPrefill = untrack(() => correction ? undefined : prefill);

  // A correction carries its evidence edits beside the creation terms.
  type SpinOffPayload = SpinOffRequest & Pick<SpinOffReplacementRequest, 'source_evidence_edits'>;

  // Pre-select the holding the screen names (#186); a correction keeps its own.
  usePrefilledPosition(untrack(() => correction ? undefined : prefill),
    () => holdings.map((holding) => ({ account_id: holding.accountID, commodity_id: holding.commodityID, key: holding.key })),
    (holding) => holding.key, () => holdingKey, (key) => { holdingKey = key; });

  const queryClient = useQueryClient();

  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  // Pre-fill once from the spin-off being corrected; later edits are the user's.
  const initialTerms = untrack(() => correction?.terms);
  const initialPercent = initialTerms
    ? basisFractionPercent(initialTerms.plan.basis_fraction_value, initialTerms.plan.basis_fraction_scale) : null;
  let effectiveOn = $state(initialTerms?.effective_on ?? entryPrefill?.transactionDate ?? '');
  let holdingKey = $state(initialTerms ? `${initialTerms.holding_account_id}:${initialTerms.commodity_id}` : '');
  let newCommodityID = $state(initialTerms ? String(initialTerms.destination_commodity_id) : '');
  // Null follows the suggested destination; a choice the user makes sticks
  // until the holding or the new instrument changes.
  let chosenDestination = $state<string | null>(!initialTerms ? null
    : initialTerms.destination_holding_account_id === initialTerms.holding_account_id
      ? 'same' : String(initialTerms.destination_holding_account_id));
  let newUnits = $state(initialTerms ? String(initialTerms.plan.ratio_numerator) : '');
  let parentUnits = $state(initialTerms ? String(initialTerms.plan.ratio_denominator) : '');
  let basisPercent = $state(initialPercent ? formatEditableAmount(initialPercent.value, initialPercent.scale) : '');
  // The recorded evidence pre-fills the ex date and reference it was entered
  // with; a correction sends only the fields changed here (#311).
  const initialEvidence = initialTerms?.source_evidence as Record<string, unknown> | undefined;
  let exDate = $state(evidenceText(initialEvidence, 'ex_date'));
  let reason = $state('');
  let sourceReference = $state(evidenceText(initialEvidence, 'reference'));
  let memo = $state(entryPrefill?.memo ?? '');
  let pending = $state(false);
  const accountsQuery = createQuery(() => investmentEntryAccountsQueryOptions(effectiveOn || ownerToday()));
  let formError = $state<unknown>(undefined);
  // The reviewed preview. Editing any field discards it, so the user always
  // records exactly the plan they saw.
  let preview = $state<{
    plan: SpinOffPlan;
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    payload: SpinOffPayload;
  } | null>(null);
  let reviewModal = $state<{ gainRefreshed: boolean } | null>(null);
  const locale = getLocale();
  // Holdings as they stood on the distribution date (#166); before a date is
  // chosen, today's.
  const holdingsDate = $derived(effectiveOn || ownerToday());
  const positionsQuery = createQuery(() => datedHoldingsQueryOptions(holdingsDate));

  const accounts = $derived((accountsQuery.data?.accounts ?? []).filter((account) =>
    isInvestmentHoldingAccount(account)));
  const instruments = $derived((instrumentsQuery.data?.instruments ?? []).filter((instrument) => instrument.status === 'active'));
  // Every long lot of the parent in one account is entitled, across every
  // cost currency its lots carry.
  const holdings = $derived.by(() => {
    const seen = new Map<string, { accountID: number; commodityID: number }>();
    for (const position of positionsQuery.data ?? []) {
      if (position.position_side !== 'long' || coefficientSign(position.quantity_value) <= 0) continue;
      if (!accounts.some((account) => account.id === position.account_id)) continue;
      const key = `${position.account_id}:${position.commodity_id}`;
      if (!seen.has(key)) seen.set(key, { accountID: position.account_id, commodityID: position.commodity_id });
    }
    return [...seen.entries()].map(([key, value]) => ({ key, ...value }));
  });
  // A corrected spin-off's parent holding stays fixed.
  const selectedHolding = $derived(correction
    ? { key: holdingKey, accountID: correction.terms.holding_account_id, commodityID: correction.terms.commodity_id }
    : holdings.find((holding) => holding.key === holdingKey));
  const newInstruments = $derived(instruments.filter((instrument) => instrument.commodity_id !== selectedHolding?.commodityID));
  const newID = $derived(Number(newCommodityID) || 0);
  // A holding account tied to one instrument cannot hold another, so the new
  // units go to an account that is untied or tied to the new instrument.
  const canHoldNew = (account: { default_commodity_id?: number | null }) =>
    account.default_commodity_id == null || account.default_commodity_id === newID;
  const sourceAccount = $derived(accounts.find((account) => account.id === selectedHolding?.accountID));
  const sameHoldingAllowed = $derived(!!sourceAccount && newID > 0 && canHoldNew(sourceAccount));
  const otherDestinations = $derived(newID > 0
    ? accounts.filter((account) => account.id !== selectedHolding?.accountID && canHoldNew(account)) : []);
  const suggestedDestination = $derived(sameHoldingAllowed ? 'same'
    : String(otherDestinations.find((account) => account.default_commodity_id === newID)?.id ?? ''));
  const destination = $derived(chosenDestination ?? suggestedDestination);
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
  const canPreview = $derived(!loading && !loadError && !!effectiveOn && !!selectedHolding && newID > 0 &&
    !!destination && !!newUnits.trim() && !!parentUnits.trim() && !!basisPercent.trim() &&
    (!correction || !!reason.trim()));
  const modalGainRows = $derived(preview?.gainImpact
    ? gainImpactRows(preview.gainImpact.changes, gainImpactCurrency(currenciesByID), locale)
    : []);


  function discardPreview() {
    preview = null;
    formError = undefined;
  }

  function resetDestination() {
    chosenDestination = null;
    discardPreview();
  }

  async function handlePreview(event: SubmitEvent) {
    event.preventDefault();
    if (!canPreview || !selectedHolding) return;
    const ratio = parseExchangeRatio(newUnits, parentUnits);
    if (!ratio.ok) {
      formError = new TranslatedFormError(m.investments_spin_off_ratio_error());
      return;
    }
    const fraction = parseBasisPercent(basisPercent);
    if (!fraction.ok) {
      formError = new TranslatedFormError(m.investments_spin_off_percent_error());
      return;
    }
    // A correction never sends back evidence it parsed: the server applies
    // the changed fields to the recorded tokens.
    const evidenceFields = { reference: sourceReference, ex_date: exDate };
    const payload: SpinOffPayload = {
      effective_on: effectiveOn,
      holding_account_id: selectedHolding.accountID,
      ...(destination !== 'same' ? { destination_holding_account_id: Number(destination) } : {}),
      commodity_id: selectedHolding.commodityID,
      destination_commodity_id: newID,
      ratio_numerator: ratio.numerator,
      ratio_denominator: ratio.denominator,
      basis_fraction_value: fraction.value,
      basis_fraction_scale: fraction.scale,
      ...(correction
        ? { source_evidence_edits: sourceEvidenceEdits(initialEvidence, evidenceFields) }
        : { source_evidence: newSourceEvidence(evidenceFields) }),
      memo: memo.trim() || undefined
    };
    pending = true;
    formError = undefined;
    try {
      await runPreview(payload);
    } catch (error) {
      preview = null;
      formError = error;
    } finally {
      pending = false;
    }
  }

  function replacementRequest(payload: SpinOffPayload): SpinOffReplacementRequest {
    return {
      reason: reason.trim(), effective_on: payload.effective_on,
      destination_holding_account_id: payload.destination_holding_account_id,
      destination_commodity_id: payload.destination_commodity_id,
      ratio_numerator: payload.ratio_numerator, ratio_denominator: payload.ratio_denominator,
      basis_fraction_value: payload.basis_fraction_value, basis_fraction_scale: payload.basis_fraction_scale,
      source_evidence_edits: payload.source_evidence_edits, memo: payload.memo
    };
  }

  async function runPreview(payload: SpinOffPayload): Promise<void> {
    const result = correction
      ? await previewSpinOffReplacement(correction.transactionID, replacementRequest(payload))
      : await previewSpinOff(payload);
    preview = {
      plan: result.plan,
      impacts: result.impact.affected_checkpoints,
      gainImpact: hasGainChanges(result.impact.gain_impact) ? result.impact.gain_impact : null,
      payload
    };
  }

  async function record(current: NonNullable<typeof preview>, confirmations: ReviewGuards) {
    if (correction) {
      await replaceSpinOff(correction.transactionID, { ...replacementRequest(current.payload), ...confirmations }, csrfToken);
    } else {
      await recordSpinOff({ ...current.payload, ...confirmations }, csrfToken);
    }
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  // The server recomputes everything at commit. A changed gain or checkpoint
  // set re-opens the review with the current figures instead of a dead-end
  // error, and the write is never retried on its own (#400).
  const review = previewReview<SpinOffPayload, NonNullable<typeof preview>>({
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
  <h2 id="spin-off-title" class="text-base font-semibold text-foreground">
    {correction ? m.transactions_investment_replace_spin_off_title() : m.investments_spin_off_title()}
  </h2>
  <p class="text-sm text-muted">{correction ? m.investments_spin_off_replace_help() : m.investments_spin_off_help()}</p>
  {#if loading}
    <p class="text-sm text-muted" role="status">{m.investments_loading()}</p>
  {:else if loadError}
    <p class="text-sm text-danger" role="alert">{m.investments_spin_off_load_error()}</p>
  {:else}
    <div class="grid gap-3 sm:grid-cols-2">
      <div>
        <label for="spin-off-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_spin_off_effective_date()}</label>
        <input id="spin-off-date" type="date" {@attach autofocus} bind:value={effectiveOn} required oninput={discardPreview}
          aria-describedby="spin-off-date-hint"
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
        <p id="spin-off-date-hint" class="mt-1 text-xs text-muted">{m.investments_spin_off_effective_date_hint()}</p>
      </div>
      <div>
        <label for="spin-off-ex-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_spin_off_ex_date()}</label>
        <input id="spin-off-ex-date" type="date" bind:value={exDate} oninput={discardPreview}
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
      </div>
    </div>
    {#if holdings.length === 0 && !correction}
      <p class="text-sm text-muted" role="status">{m.investments_spin_off_empty()}</p>
    {/if}
    <div>
      <label for="spin-off-holding" class="mb-1 block text-sm font-medium text-foreground">{m.investments_spin_off_holding()}</label>
      {#if correction}
        <p id="spin-off-holding" class="rounded-(--radius-control) border border-border bg-surface px-3 py-2 text-sm text-foreground">
          {accounts.find((account) => account.id === correction.terms.holding_account_id)?.name ?? `#${correction.terms.holding_account_id}`}
          · {instrumentLabel(correction.terms.commodity_id)}
        </p>
      {:else}
      <select id="spin-off-holding" aria-describedby="spin-off-holding-hint" bind:value={holdingKey} required onchange={resetDestination}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_exchange_select_holding()}</option>
        {#each holdings as holding (holding.key)}
          <option value={holding.key}>
            {accounts.find((account) => account.id === holding.accountID)?.name} · {instrumentLabel(holding.commodityID)}
          </option>
        {/each}
      </select>
      <p id="spin-off-holding-hint" class="mt-1 text-xs text-muted">{m.investments_dated_holdings_hint()}</p>
      {/if}
    </div>
    <div>
      <label for="spin-off-new-instrument" class="mb-1 block text-sm font-medium text-foreground">{m.investments_spin_off_new_instrument()}</label>
      <select id="spin-off-new-instrument" bind:value={newCommodityID} required onchange={resetDestination}
        aria-describedby={newInstruments.length === 0 ? 'spin-off-no-instruments' : undefined}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_exchange_select_instrument()}</option>
        {#each newInstruments as instrument (instrument.commodity_id)}
          <option value={String(instrument.commodity_id)}>{instrument.display_name}</option>
        {/each}
      </select>
      {#if newInstruments.length === 0}
        <p id="spin-off-no-instruments" class="mt-1 text-xs text-muted" role="status">{m.investments_exchange_no_instruments()}</p>
      {/if}
    </div>
    {#if selectedHolding && newID > 0}
      <div>
        <label for="spin-off-destination" class="mb-1 block text-sm font-medium text-foreground">{m.investments_exchange_destination()}</label>
        <select id="spin-off-destination" aria-describedby="spin-off-destination-hint" value={destination} required
          onchange={(event) => { chosenDestination = event.currentTarget.value; discardPreview(); }}
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
          <option value="" disabled={sameHoldingAllowed || otherDestinations.length > 0}>{m.investments_exchange_select_destination()}</option>
          {#if sameHoldingAllowed}
            <option value="same">{m.investments_exchange_same_holding({ name: sourceAccount?.name ?? '' })}</option>
          {/if}
          {#each otherDestinations as account (account.id)}
            <option value={String(account.id)}>{account.name}</option>
          {/each}
        </select>
        <p id="spin-off-destination-hint" class="mt-1 text-xs text-muted">
          {sameHoldingAllowed || otherDestinations.length > 0 ? m.investments_exchange_destination_hint() : m.investments_exchange_no_destination()}
        </p>
      </div>
    {/if}
    <fieldset class="space-y-2">
      <legend class="text-sm font-medium text-foreground">{m.investments_spin_off_ratio()}</legend>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="spin-off-new-units" class="mb-1 block text-xs text-muted">{m.investments_spin_off_ratio_new()}</label>
          <input id="spin-off-new-units" type="text" inputmode="numeric" autocomplete="off" bind:value={newUnits}
            required oninput={discardPreview} aria-describedby="spin-off-ratio-help"
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
        </div>
        <div>
          <label for="spin-off-parent-units" class="mb-1 block text-xs text-muted">{m.investments_spin_off_ratio_parent()}</label>
          <input id="spin-off-parent-units" type="text" inputmode="numeric" autocomplete="off" bind:value={parentUnits}
            required oninput={discardPreview} aria-describedby="spin-off-ratio-help"
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
        </div>
      </div>
      <p id="spin-off-ratio-help" class="text-xs text-muted">{m.investments_spin_off_ratio_help()}</p>
    </fieldset>
    <div>
      <label for="spin-off-percent" class="mb-1 block text-sm font-medium text-foreground">{m.investments_spin_off_percent()}</label>
      <div class="flex items-center gap-2">
        <input id="spin-off-percent" type="text" inputmode="decimal" autocomplete="off" bind:value={basisPercent}
          required oninput={discardPreview} aria-describedby="spin-off-percent-help"
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
        <span class="text-sm text-muted" aria-hidden="true">%</span>
      </div>
      <p id="spin-off-percent-help" class="mt-1 text-xs text-muted">{m.investments_spin_off_percent_help()}</p>
    </div>
    <div>
      <label for="spin-off-reference" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_source_reference()}</label>
      <input id="spin-off-reference" type="text" bind:value={sourceReference} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    {#if correction}
      <div>
        <label for="spin-off-reason" class="mb-1 block text-sm font-medium text-foreground">{m.investments_spin_off_replace_reason()}</label>
        <input id="spin-off-reason" type="text" bind:value={reason} maxlength="500" required oninput={discardPreview}
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
      </div>
    {/if}
    <div>
      <label for="spin-off-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
      <input id="spin-off-memo" type="text" bind:value={memo} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    {#if preview && selectedHolding}
      <section class="space-y-2 rounded-(--radius-control) border border-border p-3" aria-labelledby="spin-off-preview-title" aria-live="polite">
        <h3 id="spin-off-preview-title" class="text-sm font-semibold text-foreground">{m.investments_spin_off_preview_title()}</h3>
        <SpinOffSummary plan={preview.plan} parentName={instrumentLabel(selectedHolding.commodityID)}
          newName={instrumentLabel(newID)} {currenciesByID} />
      </section>
    {/if}
  {/if}
  <APIFormError error={formError} id="spin-off-form-error" />
  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel}
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    {#if preview}
      <button type="button" onclick={review.record} disabled={pending || !csrfToken}
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_spin_off_pending() : correction ? m.investments_spin_off_replace_submit() : m.investments_spin_off_submit()}
      </button>
    {:else}
      <button type="submit" disabled={!canPreview || pending}
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_exchange_previewing() : m.investments_spin_off_preview()}
      </button>
    {/if}
  </div>
</form>
