<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    investmentInstrumentsQueryOptions, investmentLotsQueryOptions, investmentPositionsQueryOptions,
    previewExternalTransferOut, recordExternalTransferOut, type ExternalTransferOutPlan,
    type ExternalTransferOutRequest, type GainImpact, type ReconciliationImpactResponse
  } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import { parseInternalTransferAllocations, parsePooledTransferQuantity } from './internal-transfer-amounts';
  import {
    gainAcknowledgement, gainImpactCurrency, gainImpactRows, hasGainChanges, impactNeedsReview,
    isGainAcknowledgementRefusal
  } from './gain-impact';
  import ReconciliationConfirm from './reconciliation-confirm.svelte';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import { coefficientSign } from '#lib/money/amount.ts';
  import { formatExactMoney, formatQuantity } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  // An outbound in-kind transfer (T-142): the same lot or pooled-quantity
  // selection as an internal move, with no destination in this book. The
  // basis that leaves is computed by the server from the depletion; a
  // broker's figure is kept as evidence only.
  let { csrfToken, onSaved, onCancel }: {
    csrfToken: string;
    onSaved: () => void;
    onCancel: () => void;
  } = $props();

  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const positionsQuery = createQuery(() => investmentPositionsQueryOptions());
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let effectiveOn = $state('');
  let sourceKey = $state('');
  let quantities = $state<Record<string, string>>({});
  let pooledQuantity = $state('');
  let sourceReference = $state('');
  let brokerBasis = $state('');
  let memo = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let dateInput = $state<HTMLInputElement | undefined>();
  let didFocus = false;
  // The reviewed preview. Editing any field discards it, so the user always
  // records exactly the basis they saw leave the book.
  let preview = $state<{
    plan: ExternalTransferOutPlan;
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    payload: ExternalTransferOutRequest;
  } | null>(null);
  let reviewModal = $state<{ gainRefreshed: boolean } | null>(null);

  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' }));
  const accounts = $derived((accountsQuery.data?.accounts ?? []).filter((account) =>
    account.status === 'active' && account.allows_postings &&
    (account.account_kind === 'security_holding' || account.account_kind === 'fund_holding')));
  // Long holdings only: transferring a short obligation is not modeled (#173).
  const positions = $derived((positionsQuery.data?.positions ?? []).filter((position) =>
    position.position_side === 'long' && coefficientSign(position.quantity_value) > 0 &&
    accounts.some((account) => account.id === position.account_id)));
  const sourcePosition = $derived(positions.find((position) =>
    `${position.account_id}:${position.commodity_id}:${position.cost_commodity_id}` === sourceKey));
  const pooled = $derived(sourcePosition?.transfer_basis_allocation === 'average_cost_pool');
  const selectedInstrument = $derived((instrumentsQuery.data?.instruments ?? []).find((instrument) =>
    instrument.commodity_id === sourcePosition?.commodity_id));
  const lotsQuery = createQuery(() => ({
    ...investmentLotsQueryOptions(sourcePosition?.account_id, sourcePosition?.commodity_id),
    enabled: !!sourcePosition && !pooled
  }));
  const lots = $derived((lotsQuery.data?.lots ?? []).filter((lot) =>
    lot.status === 'open' && lot.position_side === 'long' && lot.cost_commodity_id === sourcePosition?.cost_commodity_id &&
    coefficientSign(lot.remaining_quantity_value) > 0));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency) => [currency.id, currency])));
  const basisCurrency = $derived(sourcePosition ? currenciesByID.get(sourcePosition.cost_commodity_id) : undefined);
  const loading = $derived(accountsQuery.isPending || positionsQuery.isPending ||
    instrumentsQuery.isPending || currenciesQuery.isPending);
  const loadError = $derived(accountsQuery.isError || positionsQuery.isError ||
    instrumentsQuery.isError || currenciesQuery.isError);
  const quantityEntered = $derived(pooled
    ? !!pooledQuantity.trim()
    : !lotsQuery.isPending && !lotsQuery.isError && lots.some((lot) => !!quantities[String(lot.id)]?.trim()));
  const canPreview = $derived(!loading && !loadError && !!effectiveOn && !!sourcePosition &&
    !!selectedInstrument && quantityEntered);
  const modalGainRows = $derived(preview?.gainImpact
    ? gainImpactRows(preview.gainImpact.changes, gainImpactCurrency(currenciesByID), locale)
    : []);

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

  function formatBasis(value: string, scale: number): string {
    return formatExactMoney(value, scale, basisCurrency?.standard_scale ?? 2, locale);
  }

  function evidence(): ExternalTransferOutRequest['source_evidence'] {
    const reference = sourceReference.trim();
    const reported = brokerBasis.trim();
    if (!reference && !reported) return undefined;
    return {
      ...(reference ? { reference } : {}),
      ...(reported ? { broker_reported_basis: reported } : {})
    };
  }

  function buildPayload(): ExternalTransferOutRequest | null {
    if (!sourcePosition || !selectedInstrument) return null;
    const base = {
      effective_on: effectiveOn,
      source_account_id: sourcePosition.account_id,
      commodity_id: sourcePosition.commodity_id,
      cost_commodity_id: sourcePosition.cost_commodity_id,
      source_evidence: evidence(),
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
      return { ...base, quantity_value: parsed.quantity_value, quantity_scale: parsed.quantity_scale };
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

  async function runPreview(payload: ExternalTransferOutRequest): Promise<void> {
    const result = await previewExternalTransferOut(payload);
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

  async function record(override: boolean) {
    if (!preview) return;
    const acknowledgement = gainAcknowledgement(preview.gainImpact);
    await recordExternalTransferOut({
      ...preview.payload,
      ...(override ? { reconciliation_override: true } : {}),
      ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {})
    }, csrfToken);
    await invalidateInvestmentReads(queryClient);
    onSaved();
  }

  // The server recomputes everything at commit. A changed gain set re-opens
  // the review with the current figures instead of a dead-end error.
  async function commit(override: boolean) {
    if (!preview) return;
    pending = true;
    formError = undefined;
    try {
      await record(override);
    } catch (error) {
      if (isGainAcknowledgementRefusal(error) && preview) {
        try {
          await runPreview(preview.payload);
          if (preview && impactNeedsReview({ affected_checkpoints: preview.impacts, gain_impact: preview.gainImpact })) {
            reviewModal = { gainRefreshed: !!preview.gainImpact };
          }
        } catch (previewError) {
          preview = null;
          formError = previewError;
        }
      } else {
        formError = error;
      }
    } finally {
      pending = false;
    }
  }

  function handleRecord() {
    if (!preview) return;
    if (impactNeedsReview({ affected_checkpoints: preview.impacts, gain_impact: preview.gainImpact })) {
      reviewModal = { gainRefreshed: false };
      return;
    }
    void commit(false);
  }

  function confirmReview() {
    if (!preview) return;
    const override = preview.impacts.length > 0;
    reviewModal = null;
    void commit(override);
  }
</script>

{#if reviewModal && preview}
  <ReconciliationConfirm impacts={preview.impacts} gainRows={modalGainRows}
    gainRefreshed={reviewModal.gainRefreshed} {pending}
    onCancel={() => (reviewModal = null)} onConfirm={confirmReview} />
{/if}

<form onsubmit={handlePreview} class="space-y-4" aria-busy={pending}>
  <h2 id="external-transfer-out-title" class="text-base font-semibold text-foreground">{m.investments_transfer_out_title()}</h2>
  <p class="text-sm text-muted">{m.investments_transfer_out_help()}</p>
  {#if loading}
    <p class="text-sm text-muted" role="status">{m.investments_loading()}</p>
  {:else if loadError}
    <p class="text-sm text-danger" role="alert">{m.investments_transfer_load_error()}</p>
  {:else}
    {#if positions.length === 0}
      <p class="text-sm text-muted" role="status">{m.investments_transfer_out_setup_empty()}</p>
    {/if}
    <div>
      <label for="external-transfer-out-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_effective_date()}</label>
      <input id="external-transfer-out-date" type="date" bind:this={dateInput} bind:value={effectiveOn} required oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="external-transfer-out-source" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_internal_source()}</label>
      <select id="external-transfer-out-source" bind:value={sourceKey} required
        onchange={() => { quantities = {}; pooledQuantity = ''; discardPreview(); }}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_transfer_internal_select_source()}</option>
        {#each positions as position (`${position.account_id}:${position.commodity_id}:${position.cost_commodity_id}`)}
          {@const account = accounts.find((item) => item.id === position.account_id)}
          {@const instrument = (instrumentsQuery.data?.instruments ?? []).find((item) => item.commodity_id === position.commodity_id)}
          {@const currency = currenciesByID.get(position.cost_commodity_id)}
          <option value={`${position.account_id}:${position.commodity_id}:${position.cost_commodity_id}`}>
            {account?.name} · {instrument?.display_name ?? instrument?.commodity_code ?? `#${position.commodity_id}`} · {currency?.code ?? `#${position.cost_commodity_id}`}
          </option>
        {/each}
      </select>
    </div>
    {#if sourcePosition}
      {#if pooled}
        <div class="space-y-2 rounded-(--radius-control) border border-border p-3">
          <p id="external-transfer-out-pooled-help" class="text-xs text-muted">{m.investments_transfer_internal_pooled_help()}</p>
          <label for="external-transfer-out-pooled-quantity" class="block text-sm font-medium text-foreground">
            {m.investments_transfer_internal_lot_quantity()}
            <span class="block text-xs font-normal text-muted">{m.investments_transfer_internal_available()} {formatQuantity(sourcePosition.quantity_value, sourcePosition.quantity_scale, locale)}</span>
          </label>
          <input id="external-transfer-out-pooled-quantity" type="text" inputmode="decimal" autocomplete="off"
            bind:value={pooledQuantity} oninput={discardPreview} aria-describedby="external-transfer-out-pooled-help"
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
        </div>
      {:else}
        <fieldset class="space-y-3 rounded-(--radius-control) border border-border p-3">
          <legend class="px-1 text-sm font-medium text-foreground">{m.investments_transfer_internal_lots()}</legend>
          {#if lotsQuery.isPending}
            <p class="text-sm text-muted" role="status">{m.investments_loading()}</p>
          {:else if lotsQuery.isError}
            <p class="text-sm text-danger" role="alert">{m.investments_transfer_internal_lots_error()}</p>
          {:else if lots.length === 0}
            <p class="text-sm text-muted" role="status">{m.investments_transfer_internal_lots_empty()}</p>
          {:else}
            <p class="text-xs text-muted">{m.investments_transfer_internal_lots_help()}</p>
            {#each lots as lot (lot.id)}
              <div class="grid gap-2 border-t border-border pt-3 sm:grid-cols-[minmax(0,1fr)_9rem] sm:items-center">
                <label for={`external-transfer-out-lot-${lot.id}`} class="text-sm text-foreground">
                  <span class="font-medium">{m.investments_transfer_internal_lot_quantity()}</span>
                  <span class="block text-xs text-muted">#{lot.id} · {dateFormatter.format(parseISO(lot.opened_on))} · {m.investments_transfer_internal_available()} {formatQuantity(lot.remaining_quantity_value, lot.remaining_quantity_scale, locale)} · {#if lot.remaining_cost_basis_value !== null && lot.remaining_cost_basis_scale !== null}{m.investments_col_cost_basis()} {formatQuantity(lot.remaining_cost_basis_value, lot.remaining_cost_basis_scale, locale)} {basisCurrency?.code ?? ''}{:else}{m.investments_basis_unknown()}{/if}</span>
                </label>
                <input id={`external-transfer-out-lot-${lot.id}`} type="text" inputmode="decimal"
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
      <label for="external-transfer-out-reference" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_source_reference()}</label>
      <input id="external-transfer-out-reference" type="text" bind:value={sourceReference} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="external-transfer-out-broker-basis" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_out_broker_basis()}</label>
      <input id="external-transfer-out-broker-basis" type="text" inputmode="decimal" autocomplete="off" bind:value={brokerBasis}
        maxlength="100" oninput={discardPreview} aria-describedby="external-transfer-out-broker-basis-help"
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
      <p id="external-transfer-out-broker-basis-help" class="mt-1 text-xs text-muted">{m.investments_transfer_out_broker_basis_help()}</p>
    </div>
    <div>
      <label for="external-transfer-out-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
      <input id="external-transfer-out-memo" type="text" bind:value={memo} maxlength="500" oninput={discardPreview}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    {#if preview}
      <section class="space-y-2 rounded-(--radius-control) border border-border p-3" aria-labelledby="external-transfer-out-preview-title" aria-live="polite">
        <h3 id="external-transfer-out-preview-title" class="text-sm font-semibold text-foreground">{m.investments_transfer_out_preview_title()}</h3>
        <ul class="space-y-1 text-sm text-foreground">
          {#each preview.plan.links as link (link.source_lot_id)}
            {@const values = {
              lot: String(link.source_lot_id),
              quantity: formatQuantity(link.quantity_value, link.quantity_scale, locale),
              // Unknown basis has no amount: the word takes the currency slot.
              basis: link.carried_basis_value !== null && link.carried_basis_scale !== null
                ? formatBasis(link.carried_basis_value, link.carried_basis_scale) : '',
              currency: link.carried_basis_value !== null ? basisCurrency?.code ?? '' : m.investments_transfer_basis_word_unknown()
            }}
            <li class="break-words">{link.original_acquired_on
              ? m.investments_transfer_internal_link({ ...values, date: dateFormatter.format(parseISO(link.original_acquired_on)) })
              : m.investments_transfer_internal_link_date_unknown(values)}</li>
          {/each}
        </ul>
        {#if preview.plan.basis_value !== null && preview.plan.basis_scale !== null}
          <p class="text-sm font-semibold text-foreground">{m.investments_transfer_out_total({
            basis: formatBasis(preview.plan.basis_value, preview.plan.basis_scale), currency: basisCurrency?.code ?? '' })}</p>
          <p class="text-xs text-muted" role="note">{coefficientSign(preview.plan.basis_value) === 0
            ? m.investments_transfer_out_zero_note()
            : m.investments_transfer_out_bridge_note()}</p>
        {:else}
          <p class="text-sm font-semibold text-foreground">{m.investments_transfer_out_total_unknown()}</p>
          <p class="text-xs text-muted" role="note">{m.investments_transfer_out_unknown_note()}</p>
        {/if}
        {#if preview.plan.basis_allocation === 'average_cost_pool'}
          <p class="text-xs text-muted" role="note">{m.investments_transfer_internal_pooled_note()}</p>
        {/if}
      </section>
    {/if}
  {/if}
  <APIFormError error={formError} id="external-transfer-out-form-error" />
  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel}
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    {#if preview}
      <button type="button" onclick={handleRecord} disabled={pending || !csrfToken}
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
</form>
