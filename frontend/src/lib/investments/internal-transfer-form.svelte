<script lang="ts">
  import UnavailableAllocations from '#lib/investments/unavailable-allocations.svelte';
  import { unavailableAllocations, withoutAllocations } from '#lib/investments/lot-allocations.ts';
  import { useEntryActivity } from '#lib/transactions/entry-context.svelte.ts';
  import { isInvestmentHoldingAccount } from '#lib/investments/settlement-accounts.ts';
  import { investmentEntryAccountsQueryOptions } from '#lib/api/investment-entry.ts';
  import { autofocus } from '#lib/components/autofocus.ts';
  import { untrack } from 'svelte';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryOptions, datedHoldingsQueryOptions,
    previewInternalTransfer, recordInternalTransfer, type GainImpact, type InternalTransferPlan,
    type InternalTransferRequest, type ReconciliationImpactResponse
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import { parseInternalTransferAllocations, parsePooledTransferQuantity } from './internal-transfer-amounts';
  import {
    gainImpactCurrency,
    gainImpactRows,
    hasGainChanges
  } from './gain-impact';
  import { previewReview, type ReviewGuards } from './preview-review';
  import ReconciliationConfirm from './reconciliation-confirm.svelte';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import { coefficientSign } from '#lib/money/amount.ts';
  import { formatExactMoney, formatQuantity } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { usePrefilledPosition } from '#lib/investments/investment-prefill.svelte.ts';
  import { restoredPositionKey } from '#lib/investments/investment-prefill.ts';
  import type { InvestmentPrefill } from '#lib/investments/investment-prefill.ts';

  const ownerToday = useOwnerToday();

  let { csrfToken, onSaved, onCancel, prefill }: {
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
    prefill?: InvestmentPrefill;
  } = $props();
  const entryPrefill = untrack(() => prefill);

  // Pre-select the position the screen names (#186).
  usePrefilledPosition(untrack(() => prefill), () => positions,
    (position) => `${position.account_id}:${position.commodity_id}:${position.cost_commodity_id}`, () => sourceKey, (key) => { sourceKey = key; },
    restoredPositionKey(untrack(() => prefill), true));

  const queryClient = useQueryClient();

  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let effectiveOn = $state(entryPrefill?.transactionDate ?? '');
  // Holdings and lots as they stood on the transfer date, including ones a
  // later sale closed (#166). A transfer behind later history is admitted
  // by replay; the writer rechecks everything at commit (#167).
  const positionsQuery = createQuery(() => datedHoldingsQueryOptions(effectiveOn || ownerToday()));
  let sourceKey = $state('');
  let destinationAccountID = $state('');
  let quantities = $state<Record<string, string>>({});
  let pooledQuantity = $state('');
  // An average-cost source opens one pooled destination lot by default; the
  // user may keep one lot per source lot instead (T-135).
  let lineage = $state<'pooled_lot' | 'source_lots'>('pooled_lot');
  let sourceReference = $state('');
  let memo = $state(entryPrefill?.memo ?? '');
  let pending = $state(false);
  const accountsQuery = createQuery(() => investmentEntryAccountsQueryOptions(effectiveOn || ownerToday()));
  let formError = $state<unknown>(undefined);
  // New transaction's working copy and shared context (#508).
  const workingCopy = useEntryActivity('internal-transfer', {
    values: () => ({ destinationAccountID, quantities, pooledQuantity, lineage }),
    date: () => effectiveOn,
    memo: () => memo,
    reference: () => sourceReference,
    holding: () => sourcePosition?.account_id,
    instrument: () => sourcePosition?.commodity_id,
    // A position is its holding, instrument and cost currency (#508 review).
    currency: () => sourcePosition?.cost_commodity_id,
    pending: () => pending
  });
  if (workingCopy) untrack(() => {
    destinationAccountID = workingCopy.destinationAccountID ?? destinationAccountID;
    quantities = workingCopy.quantities ?? quantities;
    pooledQuantity = workingCopy.pooledQuantity ?? pooledQuantity;
    lineage = workingCopy.lineage ?? lineage;
  });
  untrack(() => { if (prefill?.reference) sourceReference = prefill.reference; });
  // The reviewed preview. Editing any field discards it, so the user always
  // records exactly the carried basis they saw.
  let preview = $state<{
    plan: InternalTransferPlan;
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    payload: InternalTransferRequest;
  } | null>(null);
  let reviewModal = $state<{ gainRefreshed: boolean } | null>(null);

  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' }));
  const accounts = $derived((accountsQuery.data?.accounts ?? []).filter((account) =>
    isInvestmentHoldingAccount(account)));
  // Long holdings only: transferring a short obligation is not modeled (#173).
  const positions = $derived((positionsQuery.data ?? []).filter((position) =>
    position.position_side === 'long' && coefficientSign(position.quantity_value) > 0 &&
    accounts.some((account) => account.id === position.account_id)));
  const sourcePosition = $derived(positions.find((position) =>
    `${position.account_id}:${position.commodity_id}:${position.cost_commodity_id}` === sourceKey));
  // An average-cost source moves a quantity out of its pool; the server
  // allocates it to lots at the pool rate (T-123).
  const pooled = $derived(sourcePosition?.transfer_basis_allocation === 'average_cost_pool');
  const selectedInstrument = $derived((instrumentsQuery.data?.instruments ?? []).find((instrument) =>
    instrument.commodity_id === sourcePosition?.commodity_id));
  const destinationAccounts = $derived(accounts.filter((account) => account.id !== sourcePosition?.account_id));
  // The dated read already carries the position's open lots at the slot.
  const lots = $derived(pooled ? [] : (sourcePosition?.lots ?? []).map((lot) => ({
    id: lot.lot_id, status: 'open', opened_on: lot.opened_on,
    remaining_quantity_value: lot.quantity_value, remaining_quantity_scale: lot.quantity_scale,
    remaining_cost_basis_value: lot.remaining_cost_basis_value, remaining_cost_basis_scale: lot.remaining_cost_basis_scale
  })));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency) => [currency.id, currency])));
  const basisCurrency = $derived(sourcePosition ? currenciesByID.get(sourcePosition.cost_commodity_id) : undefined);
  const loading = $derived(accountsQuery.isPending || positionsQuery.isPending ||
    instrumentsQuery.isPending || currenciesQuery.isPending);
  const loadError = $derived(accountsQuery.isError || positionsQuery.isError ||
    instrumentsQuery.isError || currenciesQuery.isError);
  const quantityEntered = $derived(pooled
    ? !!pooledQuantity.trim()
    : lots.some((lot) => !!quantities[String(lot.id)]?.trim()));
  // Restored quantities on lots not held on this date (#521 review).
  const unavailableLots = $derived(!loading && !pooled && !!sourcePosition ? unavailableAllocations(quantities, lots.map((lot) => lot.id)) : []);
  const canPreview = $derived(!loading && !loadError && !!effectiveOn && !!sourcePosition &&
    !!selectedInstrument && !!destinationAccountID && quantityEntered &&
    unavailableLots.length === 0);
  const modalGainRows = $derived(preview?.gainImpact
    ? gainImpactRows(preview.gainImpact.changes, gainImpactCurrency(currenciesByID), locale)
    : []);


  function discardPreview() {
    preview = null;
    formError = undefined;
  }

  function formatBasis(value: string, scale: number): string {
    return formatExactMoney(value, scale, basisCurrency?.standard_scale ?? 2, locale);
  }

  function buildPayload(): InternalTransferRequest | null {
    if (!sourcePosition || !selectedInstrument) return null;
    const base = {
      effective_on: effectiveOn,
      source_account_id: sourcePosition.account_id,
      destination_account_id: Number(destinationAccountID),
      commodity_id: sourcePosition.commodity_id,
      cost_commodity_id: sourcePosition.cost_commodity_id,
      source_evidence: sourceReference.trim() ? { reference: sourceReference.trim() } : undefined,
      memo: memo.trim() || undefined
    };
    if (pooled) {
      const parsed = parsePooledTransferQuantity(pooledQuantity, sourcePosition, selectedInstrument.quantity_scale);
      if (!parsed.ok) {
        formError = new TranslatedFormError(parsed.reason === 'exceeds_available'
          ? m.investments_transfer_internal_pooled_exceeds()
          : m.investments_transfer_internal_pooled_quantity_error());
        return null;
      }
      return {
        ...base, quantity_value: parsed.quantity_value, quantity_scale: parsed.quantity_scale,
        destination_lineage: lineage
      };
    }
    const drafts = lots.filter((lot) => !!quantities[String(lot.id)]?.trim()).map((lot) => ({
      lotID: lot.id, quantity: quantities[String(lot.id)]
    }));
    const parsed = parseInternalTransferAllocations(drafts, lots, selectedInstrument.quantity_scale);
    if (!parsed.ok) {
      formError = new TranslatedFormError(parsed.reason === 'exceeds_available'
        ? m.investments_transfer_internal_exceeds_available()
        : parsed.reason === 'empty'
          ? m.investments_transfer_internal_select_lot()
          : m.investments_transfer_internal_quantity_error());
      return null;
    }
    return { ...base, lot_allocations: parsed.allocations };
  }

  async function runPreview(payload: InternalTransferRequest): Promise<void> {
    const result = await previewInternalTransfer(payload);
    preview = {
      plan: result.plan,
      impacts: result.impact.affected_checkpoints,
      gainImpact: hasGainChanges(result.impact.gain_impact) ? result.impact.gain_impact : null,
      payload
    };
  }

  async function handlePreview(event: SubmitEvent) {
    event.preventDefault();
    if (!canPreview) return;
    formError = undefined;
    const payload = buildPayload();
    if (!payload) return;
    pending = true;
    try {
      await runPreview(payload);
    } catch (error) {
      preview = null;
      formError = error;
    } finally {
      pending = false;
    }
  }

  async function record(current: NonNullable<typeof preview>, confirmations: ReviewGuards) {
    await recordInternalTransfer({
      ...current.payload,
      ...confirmations
    }, csrfToken);
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  // The server recomputes everything at commit. A changed gain or checkpoint
  // set re-opens the review with the current figures instead of a dead-end
  // error, and the write is never retried on its own (#400).
  const review = previewReview<InternalTransferRequest, NonNullable<typeof preview>>({
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
  <h2 id="internal-transfer-title" class="text-base font-semibold text-foreground">{m.investments_transfer_internal_title()}</h2>
  <p class="text-sm text-muted">{m.investments_transfer_internal_help()}</p>
  <p class="text-xs text-muted" role="note">{m.investments_transfer_internal_wrapper_hint()}</p>
  {#if loading}
    <p class="text-sm text-muted" role="status">{m.investments_loading()}</p>
  {:else if loadError}
    <p class="text-sm text-danger" role="alert">{m.investments_transfer_load_error()}</p>
  {:else}
    {#if positions.length === 0 || accounts.length < 2}
      <p class="text-sm text-muted" role="status">{m.investments_transfer_internal_setup_empty()}</p>
    {/if}
    <div>
      <label for="internal-transfer-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_effective_date()}</label>
      <input id="internal-transfer-date" type="date" {@attach autofocus} bind:value={effectiveOn} required oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="internal-transfer-source" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_internal_source()}</label>
      <select id="internal-transfer-source" aria-describedby="internal-transfer-source-hint" bind:value={sourceKey} required
        onchange={() => { quantities = {}; pooledQuantity = ''; lineage = 'pooled_lot'; destinationAccountID = ''; discardPreview(); }}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_transfer_internal_select_source()}</option>
        {#if sourceKey && !positions.some((item) => `${item.account_id}:${item.commodity_id}:${item.cost_commodity_id}` === sourceKey)}<option value={sourceKey} disabled>{m.investments_activity_position_unavailable_option()}</option>{/if}
        {#each positions as position (`${position.account_id}:${position.commodity_id}:${position.cost_commodity_id}`)}
          {@const account = accounts.find((item) => item.id === position.account_id)}
          {@const instrument = (instrumentsQuery.data?.instruments ?? []).find((item) => item.commodity_id === position.commodity_id)}
          {@const currency = (currenciesQuery.data?.currencies ?? []).find((item) => item.id === position.cost_commodity_id)}
          <option value={`${position.account_id}:${position.commodity_id}:${position.cost_commodity_id}`}>
            {account?.name} · {instrument?.display_name ?? instrument?.commodity_code ?? `#${position.commodity_id}`} · {currency?.code ?? `#${position.cost_commodity_id}`}
          </option>
        {/each}
      </select>
      {#if positionsQuery.data && sourceKey && !positions.some((item) => `${item.account_id}:${item.commodity_id}:${item.cost_commodity_id}` === sourceKey)}<p role="alert" class="mt-1 text-sm text-danger">{m.investments_activity_position_unavailable()}</p>{/if}
      <p id="internal-transfer-source-hint" class="mt-1 text-xs text-muted">{m.investments_dated_holdings_hint()}</p>
    </div>
    {#if sourcePosition}
      <div>
        <label for="internal-transfer-destination" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_internal_destination()}</label>
        <select id="internal-transfer-destination" bind:value={destinationAccountID} required onchange={discardPreview}
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
          <option value="">{m.investments_form_select_account()}</option>
          {#each destinationAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
        </select>
      </div>
      {#if pooled}
        <div class="space-y-2 rounded-(--radius-control) border border-border p-3">
          <p id="internal-transfer-pooled-help" class="text-xs text-muted">{m.investments_transfer_internal_pooled_help()}</p>
          <label for="internal-transfer-pooled-quantity" class="block text-sm font-medium text-foreground">
            {m.investments_transfer_internal_lot_quantity()}
            <span class="block text-xs font-normal text-muted">{m.investments_transfer_internal_available()} {formatQuantity(sourcePosition.quantity_value, sourcePosition.quantity_scale, locale)}</span>
          </label>
          <input id="internal-transfer-pooled-quantity" type="text" inputmode="decimal" autocomplete="off"
            bind:value={pooledQuantity} oninput={discardPreview} aria-describedby="internal-transfer-pooled-help"
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
          <fieldset class="space-y-2 pt-1">
            <legend class="text-sm font-medium text-foreground">{m.investments_transfer_internal_lineage_legend()}</legend>
            {#each [
              { value: 'pooled_lot', label: m.investments_transfer_internal_lineage_pooled(), help: m.investments_transfer_internal_lineage_pooled_help() },
              { value: 'source_lots', label: m.investments_transfer_internal_lineage_source_lots(), help: m.investments_transfer_internal_lineage_source_lots_help() }
            ] as option (option.value)}
              <label class="flex items-start gap-2 text-sm text-foreground">
                <input type="radio" name="internal-transfer-lineage" value={option.value}
                  checked={lineage === option.value}
                  onchange={() => { lineage = option.value as 'pooled_lot' | 'source_lots'; discardPreview(); }}
                  aria-describedby={`internal-transfer-lineage-${option.value}-help`}
                  class="mt-1" />
                <span>
                  <span class="font-medium">{option.label}</span>
                  <span id={`internal-transfer-lineage-${option.value}-help`} class="block text-xs text-muted">{option.help}</span>
                </span>
              </label>
            {/each}
          </fieldset>
        </div>
      {:else}
        <fieldset class="space-y-3 rounded-(--radius-control) border border-border p-3">
          <legend class="px-1 text-sm font-medium text-foreground">{m.investments_transfer_internal_lots()}</legend>
          {#if lots.length === 0}
            <p class="text-sm text-muted" role="status">{m.investments_transfer_internal_lots_empty()}</p>
          {:else}
            <p class="text-xs text-muted">{m.investments_transfer_internal_lots_help()}</p>
            {#each lots as lot (lot.id)}
              <div class="grid gap-2 border-t border-border pt-3 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center">
                <label for={`internal-transfer-lot-${lot.id}`} class="text-sm text-foreground">
                  <span class="font-medium">{m.investments_transfer_internal_lot_quantity()}</span>
                  <span class="block text-xs text-muted">#{lot.id} · {dateFormatter.format(parseISO(lot.opened_on))} · {m.investments_transfer_internal_available()} {formatQuantity(lot.remaining_quantity_value, lot.remaining_quantity_scale, locale)} · {#if lot.remaining_cost_basis_value !== null && lot.remaining_cost_basis_scale !== null}{m.investments_col_cost_basis()} {formatQuantity(lot.remaining_cost_basis_value, lot.remaining_cost_basis_scale, locale)} {basisCurrency?.code ?? ''}{:else}{m.investments_basis_unknown()}{/if}</span>
                </label>
                <input id={`internal-transfer-lot-${lot.id}`} type="text" inputmode="decimal"
                  value={quantities[String(lot.id)] ?? ''}
                  oninput={(event) => { quantities[String(lot.id)] = event.currentTarget.value; discardPreview(); }}
                  class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
              </div>
            {/each}
          {/if}
        </fieldset>
      {/if}
    {/if}
    <div>
      <label for="internal-transfer-reference" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_source_reference()}</label>
      <input id="internal-transfer-reference" type="text" bind:value={sourceReference} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="internal-transfer-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
      <input id="internal-transfer-memo" type="text" bind:value={memo} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    {#if preview}
      <section class="space-y-2 rounded-(--radius-control) border border-border p-3" aria-labelledby="internal-transfer-preview-title" aria-live="polite">
        <h3 id="internal-transfer-preview-title" class="text-sm font-semibold text-foreground">{m.investments_transfer_internal_preview_title()}</h3>
        <ul class="space-y-1 text-sm text-foreground">
          {#each preview.plan.links as link, index (link.source_lot_id ?? `pooled-${index}`)}
            {@const values = {
              lot: String(link.source_lot_id ?? ''),
              quantity: formatQuantity(link.quantity_value, link.quantity_scale, locale),
              // Unknown basis has no amount: the word takes the currency slot.
              basis: link.carried_basis_value !== null && link.carried_basis_scale !== null
                ? formatBasis(link.carried_basis_value, link.carried_basis_scale) : '',
              currency: link.carried_basis_value !== null ? basisCurrency?.code ?? '' : m.investments_transfer_basis_word_unknown()
            }}
            {@const date = link.original_acquired_on ? dateFormatter.format(parseISO(link.original_acquired_on)) : null}
            <li class="break-words">{link.source_lot_id === null
              ? (date ? m.investments_transfer_internal_pooled_link({ ...values, date }) : m.investments_transfer_internal_pooled_link_date_unknown(values))
              : (date ? m.investments_transfer_internal_link({ ...values, date }) : m.investments_transfer_internal_link_date_unknown(values))}</li>
          {/each}
        </ul>
        {#if preview.plan.basis_allocation === 'average_cost_pool'}
          <p class="text-xs text-muted" role="note">{m.investments_transfer_internal_pooled_note()}</p>
        {/if}
      </section>
    {/if}
  {/if}
  {#if unavailableLots.length > 0}<UnavailableAllocations onRemove={() => { quantities = withoutAllocations(quantities, unavailableLots); }} />{/if}
  <APIFormError error={formError} id="internal-transfer-form-error" />
  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel}
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    {#if preview}
      <button type="button" onclick={review.record} disabled={pending || !csrfToken}
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_transfer_pending() : m.investments_transfer_submit()}
      </button>
    {:else}
      <button type="submit" disabled={!canPreview || pending}
        class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
        {pending ? m.investments_transfer_internal_previewing() : m.investments_transfer_internal_preview()}
      </button>
    {/if}
  </div>
  </fieldset>
</form>
