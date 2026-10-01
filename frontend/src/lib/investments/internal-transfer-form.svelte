<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { accountsQueryOptions } from '$lib/api/accounts';
  import { currenciesQueryOptions } from '$lib/api/currencies';
  import { forecastQueryKey } from '$lib/api/forecast';
  import { accountRegisterQueryKey, transactionsQueryKey } from '$lib/api/transactions';
  import {
    internalTransferReconciliationImpact, investmentGainsQueryKey,
    investmentInstrumentsQueryOptions, investmentLotsQueryKey, investmentLotsQueryOptions,
    investmentPositionsQueryKey, investmentPositionsQueryOptions, recordInternalTransfer,
    type InternalTransferRequest, type ReconciliationImpactResponse
  } from '$lib/api/investments';
  import { parseInternalTransferAllocations } from './internal-transfer-amounts';
  import ReconciliationConfirm from './reconciliation-confirm.svelte';
  import { TranslatedFormError } from '$lib/form-errors';
  import { coefficientSign } from '$lib/money/amount';
  import { formatQuantity } from '$lib/money/format';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';

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
  let destinationAccountID = $state('');
  let quantities = $state<Record<string, string>>({});
  let sourceReference = $state('');
  let memo = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let dateInput = $state<HTMLInputElement | undefined>();
  let didFocus = false;
  let reconciliationModal = $state<{
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    payload: InternalTransferRequest;
  } | null>(null);

  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' }));
  const accounts = $derived((accountsQuery.data?.accounts ?? []).filter((account) =>
    account.status === 'active' && account.allows_postings &&
    (account.account_kind === 'security_holding' || account.account_kind === 'fund_holding')));
  const positions = $derived((positionsQuery.data?.positions ?? []).filter((position) =>
    coefficientSign(position.quantity_value) > 0 &&
    accounts.some((account) => account.id === position.account_id)));
  const sourcePosition = $derived(positions.find((position) =>
    `${position.account_id}:${position.commodity_id}:${position.cost_commodity_id}` === sourceKey));
  const selectedInstrument = $derived((instrumentsQuery.data?.instruments ?? []).find((instrument) =>
    instrument.commodity_id === sourcePosition?.commodity_id));
  const destinationAccounts = $derived(accounts.filter((account) => account.id !== sourcePosition?.account_id));
  const lotsQuery = createQuery(() => ({
    ...investmentLotsQueryOptions(sourcePosition?.account_id, sourcePosition?.commodity_id),
    enabled: !!sourcePosition
  }));
  const lots = $derived((lotsQuery.data?.lots ?? []).filter((lot) =>
    lot.status === 'open' && lot.cost_commodity_id === sourcePosition?.cost_commodity_id &&
    coefficientSign(lot.remaining_quantity_value) > 0));
  const basisCurrency = $derived((currenciesQuery.data?.currencies ?? []).find((currency) =>
    currency.id === sourcePosition?.cost_commodity_id)?.code ?? '');
  const loading = $derived(accountsQuery.isPending || positionsQuery.isPending ||
    instrumentsQuery.isPending || currenciesQuery.isPending);
  const loadError = $derived(accountsQuery.isError || positionsQuery.isError ||
    instrumentsQuery.isError || currenciesQuery.isError);
  const canSubmit = $derived(!loading && !loadError && !lotsQuery.isPending && !lotsQuery.isError &&
    !!csrfToken && !!effectiveOn && !!sourcePosition && !!selectedInstrument &&
    !!destinationAccountID && lots.some((lot) => !!quantities[String(lot.id)]?.trim()));

  $effect(() => {
    if (!loading && !didFocus && dateInput) {
      dateInput.focus();
      didFocus = true;
    }
  });

  async function post(payload: InternalTransferRequest) {
    await recordInternalTransfer(payload, csrfToken);
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: investmentPositionsQueryKey }),
      queryClient.invalidateQueries({ queryKey: investmentLotsQueryKey }),
      queryClient.invalidateQueries({ queryKey: investmentGainsQueryKey }),
      queryClient.invalidateQueries({ queryKey: forecastQueryKey }),
      queryClient.invalidateQueries({ queryKey: transactionsQueryKey }),
      queryClient.invalidateQueries({ queryKey: accountRegisterQueryKey })
    ]);
    onSaved();
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    if (!canSubmit || !sourcePosition || !selectedInstrument) return;
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
      return;
    }
    const payload: InternalTransferRequest = {
      effective_on: effectiveOn,
      source_account_id: sourcePosition.account_id,
      destination_account_id: Number(destinationAccountID),
      commodity_id: sourcePosition.commodity_id,
      cost_commodity_id: sourcePosition.cost_commodity_id,
      lot_allocations: parsed.allocations,
      source_evidence: sourceReference.trim() ? { reference: sourceReference.trim() } : undefined,
      memo: memo.trim() || undefined
    };
    pending = true;
    formError = undefined;
    try {
      const impact = await internalTransferReconciliationImpact(payload);
      if (impact.affected_checkpoints.length > 0) {
        reconciliationModal = { impacts: impact.affected_checkpoints, payload };
        return;
      }
      await post(payload);
    } catch (error) {
      formError = error;
    } finally {
      pending = false;
    }
  }

  async function confirmOverride() {
    if (!reconciliationModal) return;
    const payload = reconciliationModal.payload;
    reconciliationModal = null;
    pending = true;
    formError = undefined;
    try {
      await post({ ...payload, reconciliation_override: true });
    } catch (error) {
      formError = error;
    } finally {
      pending = false;
    }
  }
</script>

{#if reconciliationModal}
  <ReconciliationConfirm impacts={reconciliationModal.impacts} {pending}
    onCancel={() => (reconciliationModal = null)} onConfirm={confirmOverride} />
{/if}

<form onsubmit={handleSubmit} class="space-y-4" aria-busy={pending}>
  <h2 id="internal-transfer-title" class="text-base font-semibold text-foreground">{m.investments_transfer_internal_title()}</h2>
  <p class="text-sm text-muted">{m.investments_transfer_internal_help()}</p>
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
      <input id="internal-transfer-date" type="date" bind:this={dateInput} bind:value={effectiveOn} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="internal-transfer-source" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_internal_source()}</label>
      <select id="internal-transfer-source" bind:value={sourceKey} required
        onchange={() => { quantities = {}; destinationAccountID = ''; }}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_transfer_internal_select_source()}</option>
        {#each positions as position (`${position.account_id}:${position.commodity_id}:${position.cost_commodity_id}`)}
          {@const account = accounts.find((item) => item.id === position.account_id)}
          {@const instrument = (instrumentsQuery.data?.instruments ?? []).find((item) => item.commodity_id === position.commodity_id)}
          {@const currency = (currenciesQuery.data?.currencies ?? []).find((item) => item.id === position.cost_commodity_id)}
          <option value={`${position.account_id}:${position.commodity_id}:${position.cost_commodity_id}`}>
            {account?.name} · {instrument?.display_name ?? instrument?.commodity_code ?? `#${position.commodity_id}`} · {currency?.code ?? `#${position.cost_commodity_id}`}
          </option>
        {/each}
      </select>
    </div>
    {#if sourcePosition}
      <div>
        <label for="internal-transfer-destination" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_internal_destination()}</label>
        <select id="internal-transfer-destination" bind:value={destinationAccountID} required
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
          <option value="">{m.investments_form_select_account()}</option>
          {#each destinationAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
        </select>
      </div>
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
              <label for={`internal-transfer-lot-${lot.id}`} class="text-sm text-foreground">
                <span class="font-medium">{m.investments_transfer_internal_lot_quantity()}</span>
                <span class="block text-xs text-muted">#{lot.id} · {dateFormatter.format(parseISO(lot.opened_on))} · {m.investments_transfer_internal_available()} {formatQuantity(lot.remaining_quantity_value, lot.remaining_quantity_scale, locale)} · {m.investments_col_cost_basis()} {lot.remaining_cost_basis_value !== null && lot.remaining_cost_basis_scale !== null ? formatQuantity(lot.remaining_cost_basis_value, lot.remaining_cost_basis_scale, locale) : m.investments_basis_unknown()} {basisCurrency}</span>
              </label>
              <input id={`internal-transfer-lot-${lot.id}`} type="text" inputmode="decimal"
                disabled={lot.basis_knowledge === 'unknown'}
                value={quantities[String(lot.id)] ?? ''}
                oninput={(event) => { quantities[String(lot.id)] = event.currentTarget.value; }}
                class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
            </div>
          {/each}
        {/if}
      </fieldset>
    {/if}
    <div>
      <label for="internal-transfer-reference" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_source_reference()}</label>
      <input id="internal-transfer-reference" type="text" bind:value={sourceReference} maxlength="500"
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="internal-transfer-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
      <input id="internal-transfer-memo" type="text" bind:value={memo} maxlength="500"
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
  {/if}
  <APIFormError error={formError} id="internal-transfer-form-error" />
  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel}
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    <button type="submit" disabled={!canSubmit || pending}
      class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
      {pending ? m.investments_transfer_pending() : m.investments_transfer_submit()}
    </button>
  </div>
</form>
