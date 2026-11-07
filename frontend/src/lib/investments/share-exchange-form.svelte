<script lang="ts">
  import { useEntryActivity } from '#lib/transactions/entry-context.svelte.ts';
  import { isInvestmentHoldingAccount } from '#lib/investments/settlement-accounts.ts';
  import { investmentEntryAccountsQueryOptions } from '#lib/api/investment-entry.ts';
  import { autofocus } from '#lib/components/autofocus.ts';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryOptions, datedHoldingsQueryOptions, previewShareExchange, recordShareExchange,
    previewShareExchangeReplacement, replaceShareExchange,
    type GainImpact, type InvestmentCorrectionShareExchangeTerms, type ReconciliationImpactResponse,
    type ShareExchangePlan, type ShareExchangeReplacementRequest, type ShareExchangeRequest
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import {
    gainImpactCurrency,
    gainImpactRows,
    hasGainChanges
  } from './gain-impact';
  import { previewReview, type ReviewGuards } from './preview-review';
  import ReconciliationConfirm from './reconciliation-confirm.svelte';
  import ShareExchangeSummary from './share-exchange-summary.svelte';
  import { evidenceText, newSourceEvidence, sourceEvidenceEdits } from './source-evidence';
  import { parseExchangeRatio } from './split-ratio';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import { coefficientSign } from '#lib/money/amount.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { usePrefilledPosition } from '#lib/investments/investment-prefill.svelte.ts';
  import type { InvestmentPrefill } from '#lib/investments/investment-prefill.ts';

  const ownerToday = useOwnerToday();

  // Share exchange entry (#178): a merger, fund merger or class conversion
  // turns the whole long holding of one instrument into another at an exact
  // ratio. The server plans every lot; the form previews that plan and records
  // exactly what was previewed. With correction set, the form replaces that
  // posted exchange (#179): the exchanged holding is fixed, the current terms
  // are pre-filled and a reason is required.
  let { csrfToken, onSaved, onCancel, correction, prefill }: {
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
    prefill?: InvestmentPrefill;
    correction?: { transactionID: number; terms: InvestmentCorrectionShareExchangeTerms };
  } = $props();
  const entryPrefill = untrack(() => correction ? undefined : prefill);

  // A correction carries its evidence edits beside the creation terms.
  type ExchangePayload = ShareExchangeRequest & Pick<ShareExchangeReplacementRequest, 'source_evidence_edits'>;

  // Pre-select the holding the screen names (#186); a correction keeps its own.
  usePrefilledPosition(untrack(() => correction ? undefined : prefill),
    () => holdings.map((holding) => ({ account_id: holding.accountID, commodity_id: holding.commodityID, key: holding.key })),
    (holding) => holding.key, () => holdingKey, (key) => { holdingKey = key; });

  const queryClient = useQueryClient();

  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  // Pre-fill once from the exchange being corrected; later edits are the user's.
  const initialTerms = untrack(() => correction?.terms);
  let effectiveOn = $state(initialTerms?.effective_on ?? entryPrefill?.transactionDate ?? '');
  let holdingKey = $state(initialTerms ? `${initialTerms.holding_account_id}:${initialTerms.commodity_id}` : '');
  let newCommodityID = $state(initialTerms ? String(initialTerms.destination_commodity_id) : '');
  // Null follows the suggested destination; a choice the user makes sticks
  // until the holding or the new instrument changes.
  let chosenDestination = $state<string | null>(!initialTerms ? null
    : initialTerms.destination_holding_account_id === initialTerms.holding_account_id
      ? 'same' : String(initialTerms.destination_holding_account_id));
  let newUnits = $state(initialTerms ? String(initialTerms.plan.ratio_numerator) : '');
  let oldUnits = $state(initialTerms ? String(initialTerms.plan.ratio_denominator) : '');
  let reason = $state('');
  // The recorded evidence pre-fills the reference it was entered with; a
  // correction sends only a changed reference, so a new reference no longer
  // replaces the rest of the recorded evidence (#311).
  const initialEvidence = initialTerms?.source_evidence as Record<string, unknown> | undefined;
  let sourceReference = $state(evidenceText(initialEvidence, 'reference'));
  let memo = $state(entryPrefill?.memo ?? '');
  let pending = $state(false);
  const accountsQuery = createQuery(() => investmentEntryAccountsQueryOptions(effectiveOn || ownerToday()));
  let formError = $state<unknown>(undefined);
  // New transaction's working copy and shared context (#508).
  const workingCopy = useEntryActivity('share-exchange', {
    correction: !!initialTerms,
    values: () => ({ newCommodityID, chosenDestination, newUnits, oldUnits }),
    date: () => effectiveOn,
    memo: () => memo,
    reference: () => sourceReference,
    holding: () => selectedHolding?.accountID,
    instrument: () => selectedHolding?.commodityID,
    pending: () => pending
  });
  if (workingCopy) untrack(() => {
    newCommodityID = workingCopy.newCommodityID ?? newCommodityID;
    chosenDestination = workingCopy.chosenDestination ?? chosenDestination;
    newUnits = workingCopy.newUnits ?? newUnits;
    oldUnits = workingCopy.oldUnits ?? oldUnits;
  });
  untrack(() => { if (!initialTerms && prefill?.reference) sourceReference = prefill.reference; });
  // The reviewed preview. Editing any field discards it, so the user always
  // records exactly the plan they saw.
  let preview = $state<{
    plan: ShareExchangePlan;
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    payload: ExchangePayload;
  } | null>(null);
  let reviewModal = $state<{ gainRefreshed: boolean } | null>(null);
  // Holdings as they stood on the exchange date (#166); before a date is
  // chosen, today's.
  const holdingsDate = $derived(effectiveOn || ownerToday());
  const positionsQuery = createQuery(() => datedHoldingsQueryOptions(holdingsDate));

  const locale = $derived(getLocale());
  const accounts = $derived((accountsQuery.data?.accounts ?? []).filter((account) =>
    isInvestmentHoldingAccount(account)));
  const instruments = $derived((instrumentsQuery.data?.instruments ?? []).filter((instrument) => instrument.status === 'active'));
  // An exchange takes the whole long holding of an instrument in one account,
  // across every cost currency its lots carry.
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
  // A corrected exchange's old holding was emptied by it; it stays fixed.
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
    !!destination && !!newUnits.trim() && !!oldUnits.trim() && (!correction || !!reason.trim()));
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

  function replacementRequest(payload: ExchangePayload): ShareExchangeReplacementRequest {
    return {
      reason: reason.trim(), effective_on: payload.effective_on,
      destination_holding_account_id: payload.destination_holding_account_id,
      destination_commodity_id: payload.destination_commodity_id,
      ratio_numerator: payload.ratio_numerator, ratio_denominator: payload.ratio_denominator,
      source_evidence_edits: payload.source_evidence_edits, memo: payload.memo
    };
  }

  async function runPreview(payload: ExchangePayload): Promise<void> {
    const result = correction
      ? await previewShareExchangeReplacement(correction.transactionID, replacementRequest(payload))
      : await previewShareExchange(payload);
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
    const ratio = parseExchangeRatio(newUnits, oldUnits);
    if (!ratio.ok) {
      formError = new TranslatedFormError(m.investments_exchange_ratio_error());
      return;
    }
    pending = true;
    formError = undefined;
    try {
      await runPreview({
        effective_on: effectiveOn,
        holding_account_id: selectedHolding.accountID,
        ...(destination !== 'same' ? { destination_holding_account_id: Number(destination) } : {}),
        commodity_id: selectedHolding.commodityID,
        destination_commodity_id: newID,
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
      await replaceShareExchange(correction.transactionID, { ...replacementRequest(current.payload), ...confirmations }, csrfToken);
    } else {
      await recordShareExchange({ ...current.payload, ...confirmations }, csrfToken);
    }
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  // The server recomputes everything at commit. A changed gain or checkpoint
  // set re-opens the review with the current figures instead of a dead-end
  // error, and the write is never retried on its own (#400).
  const review = previewReview<ExchangePayload, NonNullable<typeof preview>>({
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
  <h2 id="share-exchange-title" class="text-base font-semibold text-foreground">
    {correction ? m.transactions_investment_replace_exchange_title() : m.investments_exchange_title()}
  </h2>
  <p class="text-sm text-muted">{correction ? m.investments_exchange_replace_help() : m.investments_exchange_help()}</p>
  {#if loading}
    <p class="text-sm text-muted" role="status">{m.investments_loading()}</p>
  {:else if loadError}
    <p class="text-sm text-danger" role="alert">{m.investments_exchange_load_error()}</p>
  {:else}
    <div>
      <label for="exchange-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_exchange_effective_date()}</label>
      <input id="exchange-date" type="date" {@attach autofocus} bind:value={effectiveOn} required oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    {#if holdings.length === 0 && !correction}
      <p class="text-sm text-muted" role="status">{m.investments_exchange_empty()}</p>
    {/if}
    <div>
      <label for="exchange-holding" class="mb-1 block text-sm font-medium text-foreground">{m.investments_exchange_holding()}</label>
      {#if correction}
        <p id="exchange-holding" class="rounded-(--radius-control) border border-border bg-surface px-3 py-2 text-sm text-foreground">
          {accounts.find((account) => account.id === correction.terms.holding_account_id)?.name ?? `#${correction.terms.holding_account_id}`}
          · {instrumentLabel(correction.terms.commodity_id)}
        </p>
      {:else}
      <select id="exchange-holding" aria-describedby="exchange-holding-hint" bind:value={holdingKey} required onchange={resetDestination}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_exchange_select_holding()}</option>
        {#each holdings as holding (holding.key)}
          <option value={holding.key}>
            {accounts.find((account) => account.id === holding.accountID)?.name} · {instrumentLabel(holding.commodityID)}
          </option>
        {/each}
      </select>
      <p id="exchange-holding-hint" class="mt-1 text-xs text-muted">{m.investments_dated_holdings_hint()}</p>
      {/if}
    </div>
    <div>
      <label for="exchange-new-instrument" class="mb-1 block text-sm font-medium text-foreground">{m.investments_exchange_new_instrument()}</label>
      <select id="exchange-new-instrument" bind:value={newCommodityID} required onchange={resetDestination}
        aria-describedby={newInstruments.length === 0 ? 'exchange-no-instruments' : undefined}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_exchange_select_instrument()}</option>
        {#each newInstruments as instrument (instrument.commodity_id)}
          <option value={String(instrument.commodity_id)}>{instrument.display_name}</option>
        {/each}
      </select>
      {#if newInstruments.length === 0}
        <p id="exchange-no-instruments" class="mt-1 text-xs text-muted" role="status">{m.investments_exchange_no_instruments()}</p>
      {/if}
    </div>
    {#if selectedHolding && newID > 0}
      <div>
        <label for="exchange-destination" class="mb-1 block text-sm font-medium text-foreground">{m.investments_exchange_destination()}</label>
        <select id="exchange-destination" aria-describedby="exchange-destination-hint" value={destination} required
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
        <p id="exchange-destination-hint" class="mt-1 text-xs text-muted">
          {sameHoldingAllowed || otherDestinations.length > 0 ? m.investments_exchange_destination_hint() : m.investments_exchange_no_destination()}
        </p>
      </div>
    {/if}
    <fieldset class="space-y-2">
      <legend class="text-sm font-medium text-foreground">{m.investments_exchange_ratio()}</legend>
      <div class="grid grid-cols-2 gap-3">
        <div>
          <label for="exchange-new-units" class="mb-1 block text-xs text-muted">{m.investments_exchange_ratio_new()}</label>
          <input id="exchange-new-units" type="text" inputmode="numeric" autocomplete="off" bind:value={newUnits}
            required oninput={discardPreview} aria-describedby="exchange-ratio-help"
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
        </div>
        <div>
          <label for="exchange-old-units" class="mb-1 block text-xs text-muted">{m.investments_exchange_ratio_old()}</label>
          <input id="exchange-old-units" type="text" inputmode="numeric" autocomplete="off" bind:value={oldUnits}
            required oninput={discardPreview} aria-describedby="exchange-ratio-help"
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
        </div>
      </div>
      <p id="exchange-ratio-help" class="text-xs text-muted">{m.investments_exchange_ratio_help()}</p>
    </fieldset>
    <div>
      <label for="exchange-reference" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_source_reference()}</label>
      <input id="exchange-reference" type="text" bind:value={sourceReference} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    {#if correction}
      <div>
        <label for="exchange-reason" class="mb-1 block text-sm font-medium text-foreground">{m.investments_exchange_replace_reason()}</label>
        <input id="exchange-reason" type="text" bind:value={reason} maxlength="500" required oninput={discardPreview}
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
      </div>
    {/if}
    <div>
      <label for="exchange-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
      <input id="exchange-memo" type="text" bind:value={memo} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    {#if preview && selectedHolding}
      <section class="space-y-2 rounded-(--radius-control) border border-border p-3" aria-labelledby="exchange-preview-title" aria-live="polite">
        <h3 id="exchange-preview-title" class="text-sm font-semibold text-foreground">{m.investments_exchange_preview_title()}</h3>
        <ShareExchangeSummary plan={preview.plan} oldName={instrumentLabel(selectedHolding.commodityID)}
          newName={instrumentLabel(newID)} {currenciesByID} />
      </section>
    {/if}
  {/if}
  <APIFormError error={formError} id="share-exchange-form-error" />
  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel}
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    {#if preview}
      <button type="button" onclick={review.record} disabled={pending || !csrfToken}
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_exchange_pending() : correction ? m.investments_exchange_replace_submit() : m.investments_exchange_submit()}
      </button>
    {:else}
      <button type="submit" disabled={!canPreview || pending}
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_exchange_previewing() : m.investments_exchange_preview()}
      </button>
    {/if}
  </div>
  </fieldset>
</form>
