<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryOptions, datedHoldingsQueryOptions, previewSpinOff, recordSpinOff,
    type ReconciliationImpactResponse, type SpinOffPlan, type SpinOffRequest
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import ReconciliationConfirm from './reconciliation-confirm.svelte';
  import SpinOffSummary from './spin-off-summary.svelte';
  import { parseBasisPercent, parseExchangeRatio } from './split-ratio';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import { coefficientSign } from '#lib/money/amount.ts';
  import { m } from '#lib/paraglide/messages.js';

  // Spin-off entry (#180): the parent holding keeps its units, a new
  // instrument is distributed to it, and the issuer's published share of the
  // basis moves to the new lots. The server plans every lot; the form previews
  // that plan and records exactly what was previewed. Correction and
  // backdating arrive with #183.
  let { csrfToken, onSaved, onCancel }: {
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
  } = $props();

  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let effectiveOn = $state('');
  let holdingKey = $state('');
  let newCommodityID = $state('');
  // Null follows the suggested destination; a choice the user makes sticks
  // until the holding or the new instrument changes.
  let chosenDestination = $state<string | null>(null);
  let newUnits = $state('');
  let parentUnits = $state('');
  let basisPercent = $state('');
  let exDate = $state('');
  let sourceReference = $state('');
  let memo = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let dateInput = $state<HTMLInputElement | undefined>();
  let didFocus = false;
  // The reviewed preview. Editing any field discards it, so the user always
  // records exactly the plan they saw.
  let preview = $state<{
    plan: SpinOffPlan;
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    payload: SpinOffRequest;
  } | null>(null);
  let reviewOpen = $state(false);
  // Holdings as they stood on the distribution date (#166); before a date is
  // chosen, today's.
  const holdingsDate = $derived(effectiveOn || new Date().toISOString().slice(0, 10));
  const positionsQuery = createQuery(() => datedHoldingsQueryOptions(holdingsDate));

  const accounts = $derived((accountsQuery.data?.accounts ?? []).filter((account) =>
    account.status === 'active' && account.allows_postings &&
    (account.account_kind === 'security_holding' || account.account_kind === 'fund_holding')));
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
  const selectedHolding = $derived(holdings.find((holding) => holding.key === holdingKey));
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
    !!destination && !!newUnits.trim() && !!parentUnits.trim() && !!basisPercent.trim());

  $effect(() => {
    if (!loading && !didFocus && dateInput) {
      dateInput.focus();
      didFocus = true;
    }
  });

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
    const evidence: Record<string, string> = {};
    if (sourceReference.trim()) evidence.reference = sourceReference.trim();
    if (exDate) evidence.ex_date = exDate;
    const payload: SpinOffRequest = {
      effective_on: effectiveOn,
      holding_account_id: selectedHolding.accountID,
      ...(destination !== 'same' ? { destination_holding_account_id: Number(destination) } : {}),
      commodity_id: selectedHolding.commodityID,
      destination_commodity_id: newID,
      ratio_numerator: ratio.numerator,
      ratio_denominator: ratio.denominator,
      basis_fraction_value: fraction.value,
      basis_fraction_scale: fraction.scale,
      source_evidence: Object.keys(evidence).length > 0 ? evidence : undefined,
      memo: memo.trim() || undefined
    };
    pending = true;
    formError = undefined;
    try {
      const result = await previewSpinOff(payload);
      preview = { plan: result.plan, impacts: result.impact.affected_checkpoints, payload };
    } catch (error) {
      preview = null;
      formError = error;
    } finally {
      pending = false;
    }
  }

  // The server recomputes everything at commit; a moved holding is refused.
  async function commit(override: boolean) {
    if (!preview) return;
    pending = true;
    formError = undefined;
    try {
      await recordSpinOff({ ...preview.payload, ...(override ? { reconciliation_override: true } : {}) }, csrfToken);
      await invalidateInvestmentReads(queryClient);
      onSaved();
    } catch (error) {
      formError = error;
    } finally {
      pending = false;
    }
  }

  function handleRecord() {
    if (!preview) return;
    if (preview.impacts.length > 0) {
      reviewOpen = true;
      return;
    }
    void commit(false);
  }

  function confirmReview() {
    reviewOpen = false;
    void commit(true);
  }
</script>

{#if reviewOpen && preview}
  <ReconciliationConfirm impacts={preview.impacts} gainRows={[]} gainRefreshed={false} {pending}
    onCancel={() => (reviewOpen = false)} onConfirm={confirmReview} />
{/if}

<form onsubmit={handlePreview} class="space-y-4" aria-busy={pending}>
  <h2 id="spin-off-title" class="text-base font-semibold text-foreground">{m.investments_spin_off_title()}</h2>
  <p class="text-sm text-muted">{m.investments_spin_off_help()}</p>
  {#if loading}
    <p class="text-sm text-muted" role="status">{m.investments_loading()}</p>
  {:else if loadError}
    <p class="text-sm text-danger" role="alert">{m.investments_spin_off_load_error()}</p>
  {:else}
    <div class="grid gap-3 sm:grid-cols-2">
      <div>
        <label for="spin-off-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_spin_off_effective_date()}</label>
        <input id="spin-off-date" type="date" bind:this={dateInput} bind:value={effectiveOn} required oninput={discardPreview}
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
    {#if holdings.length === 0}
      <p class="text-sm text-muted" role="status">{m.investments_spin_off_empty()}</p>
    {/if}
    <div>
      <label for="spin-off-holding" class="mb-1 block text-sm font-medium text-foreground">{m.investments_spin_off_holding()}</label>
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
      <button type="button" onclick={handleRecord} disabled={pending || !csrfToken}
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_spin_off_pending() : m.investments_spin_off_submit()}
      </button>
    {:else}
      <button type="submit" disabled={!canPreview || pending}
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_exchange_previewing() : m.investments_spin_off_preview()}
      </button>
    {/if}
  </div>
</form>
