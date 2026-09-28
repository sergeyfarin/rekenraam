<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { accountsQueryOptions } from '$lib/api/accounts';
  import { currenciesQueryOptions } from '$lib/api/currencies';
  import { forecastQueryKey } from '$lib/api/forecast';
  import { accountRegisterQueryKey, transactionsQueryKey } from '$lib/api/transactions';
  import { TranslatedFormError } from '$lib/form-errors';
  import {
    externalTransferInReconciliationImpact,
    investmentGainsQueryKey,
    investmentInstrumentsQueryOptions,
    investmentLotsQueryKey,
    investmentPositionsQueryKey,
    recordExternalTransferIn,
    type ExternalTransferInRequest,
    type ReconciliationImpactResponse
  } from '$lib/api/investments';
  import { parseTransferInAmounts } from '$lib/investments/form-amounts';
  import ReconciliationConfirm from '$lib/investments/reconciliation-confirm.svelte';
  import { m } from '$lib/paraglide/messages.js';

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
  let instrumentID = $state('');
  let holdingAccountID = $state('');
  let quantityStr = $state('');
  let carriedBasisStr = $state('');
  let costCommodityID = $state('');
  let originalAcquiredOn = $state('');
  let sourceReference = $state('');
  let memo = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let dateInput = $state<HTMLInputElement | undefined>();
  let didFocus = false;
  let reconciliationModal = $state<{
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    payload: ExternalTransferInRequest;
  } | null>(null);

  const instruments = $derived((instrumentsQuery.data?.instruments ?? []).filter((instrument) => instrument.status === 'active'));
  const selectedInstrument = $derived(instruments.find((instrument) => String(instrument.commodity_id) === instrumentID));
  const holdingAccounts = $derived((accountsQuery.data?.accounts ?? []).filter((account) =>
    account.status === 'active' && account.allows_postings &&
    (account.account_kind === 'security_holding' || account.account_kind === 'fund_holding')));
  const currencies = $derived((currenciesQuery.data?.currencies ?? []).filter((currency) => currency.status === 'active'));
  const loading = $derived(accountsQuery.isPending || instrumentsQuery.isPending || currenciesQuery.isPending);
  const loadError = $derived(accountsQuery.isError || instrumentsQuery.isError || currenciesQuery.isError);
  const canSubmit = $derived(
    !loading && !loadError && !!csrfToken && !!effectiveOn && !!selectedInstrument &&
    !!holdingAccountID && !!quantityStr.trim() && !!carriedBasisStr.trim() && !!costCommodityID
  );

  $effect(() => {
    if (!loading && !didFocus && dateInput) {
      dateInput.focus();
      didFocus = true;
    }
  });

  async function post(payload: ExternalTransferInRequest) {
    await recordExternalTransferIn(payload, csrfToken);
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
    if (!canSubmit || !selectedInstrument) return;
    const amounts = parseTransferInAmounts({
      quantityStr, carriedBasisStr, quantityMaxScale: selectedInstrument.quantity_scale
    });
    if (!amounts.ok) {
      formError = new TranslatedFormError(amounts.reason === 'too_large'
        ? m.investments_form_amount_too_large()
        : amounts.field === 'quantity'
          ? m.investments_transfer_quantity_error()
          : m.investments_transfer_basis_error());
      return;
    }
    if (originalAcquiredOn && originalAcquiredOn > effectiveOn) {
      formError = new TranslatedFormError(m.investments_transfer_original_date_error());
      return;
    }
    const payload: ExternalTransferInRequest = {
      effective_on: effectiveOn,
      holding_account_id: Number(holdingAccountID),
      commodity_id: selectedInstrument.commodity_id,
      quantity_value: amounts.values.quantity.value,
      quantity_scale: amounts.values.quantity.scale,
      carried_basis_value: amounts.values.carriedBasis.value,
      carried_basis_scale: amounts.values.carriedBasis.scale,
      cost_commodity_id: Number(costCommodityID),
      original_acquired_on: originalAcquiredOn || undefined,
      source_evidence: sourceReference.trim() ? { reference: sourceReference.trim() } : undefined,
      memo: memo.trim() || undefined
    };
    pending = true;
    formError = undefined;
    try {
      const impact = await externalTransferInReconciliationImpact(payload);
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
  <h2 id="external-transfer-in-title" class="text-base font-semibold text-foreground">
    {m.investments_transfer_in_title()}
  </h2>
  <p class="text-sm text-muted">{m.investments_transfer_in_help()}</p>

  {#if loading}
    <p class="text-sm text-muted" role="status">{m.investments_loading()}</p>
  {:else if loadError}
    <p class="text-sm text-danger" role="alert">{m.investments_transfer_load_error()}</p>
  {:else}
    {#if instruments.length === 0 || holdingAccounts.length === 0 || currencies.length === 0}
      <p class="text-sm text-muted" role="status">{m.investments_transfer_setup_empty()}</p>
    {/if}
    <div>
      <label for="transfer-in-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_effective_date()}</label>
      <input id="transfer-in-date" type="date" bind:this={dateInput} bind:value={effectiveOn} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="transfer-in-instrument" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_instrument()}</label>
      <select id="transfer-in-instrument" bind:value={instrumentID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_transfer_select_instrument()}</option>
        {#each instruments as instrument (instrument.commodity_id)}
          <option value={String(instrument.commodity_id)}>{instrument.display_name} ({instrument.symbol ?? instrument.commodity_code})</option>
        {/each}
      </select>
    </div>
    <div>
      <label for="transfer-in-account" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_holding_account()}</label>
      <select id="transfer-in-account" bind:value={holdingAccountID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_form_select_account()}</option>
        {#each holdingAccounts as account (account.id)}<option value={String(account.id)}>{account.name}</option>{/each}
      </select>
    </div>
    <div>
      <label for="transfer-in-quantity" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_quantity()}</label>
      <input id="transfer-in-quantity" type="text" inputmode="decimal" bind:value={quantityStr} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
    </div>
    <div>
      <label for="transfer-in-basis" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_carried_basis()}</label>
      <input id="transfer-in-basis" type="text" inputmode="decimal" bind:value={carriedBasisStr} required
        aria-describedby="transfer-in-basis-help"
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-mono text-foreground" />
      <p id="transfer-in-basis-help" class="mt-1 text-xs text-muted">{m.investments_transfer_basis_help()}</p>
    </div>
    <div>
      <label for="transfer-in-currency" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_basis_currency()}</label>
      <select id="transfer-in-currency" bind:value={costCommodityID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        <option value="">{m.investments_transfer_select_currency()}</option>
        {#each currencies as currency (currency.id)}<option value={String(currency.id)}>{currency.code}</option>{/each}
      </select>
    </div>
    <div>
      <label for="transfer-in-original-date" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_original_date()}</label>
      <input id="transfer-in-original-date" type="date" bind:value={originalAcquiredOn}
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="transfer-in-reference" class="mb-1 block text-sm font-medium text-foreground">{m.investments_transfer_source_reference()}</label>
      <input id="transfer-in-reference" type="text" bind:value={sourceReference} maxlength="500"
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="transfer-in-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
      <input id="transfer-in-memo" type="text" bind:value={memo} maxlength="500"
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
  {/if}

  <APIFormError error={formError} id="transfer-in-form-error" />
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
