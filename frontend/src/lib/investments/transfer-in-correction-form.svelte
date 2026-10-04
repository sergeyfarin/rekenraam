<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { accountsQueryOptions, type AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import { forecastQueryKey } from '#lib/api/forecast.ts';
  import { accountRegisterQueryKey, transactionsQueryKey } from '#lib/api/transactions.ts';
  import { formatLedgerAmount } from '#lib/money/amount.ts';
  import { parseTransferInAmounts } from '#lib/investments/form-amounts.ts';
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
    investmentGainsQueryKey,
    investmentLotsQueryKey,
    investmentPositionsQueryKey,
    previewTransferInReplacement,
    replaceTransferIn,
    type ExternalTransferInRequest,
    type GainImpact,
    type InvestmentCorrectionTransferTerms,
    type ReconciliationImpactResponse
  } from '#lib/api/investments.ts';

  // Replaces a posted external transfer in (T-119). The security and basis
  // currency are fixed; the date, holding, quantity, carried basis and
  // original acquisition date may change. The inverse and the corrected
  // transfer both post, so the equity bridge difference is appended.
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
  const locale = getLocale();
  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let reason = $state('');
  let effectiveOn = $state(source.effective_on);
  let holdingAccountID = $state(String(source.destination_account_id));
  let quantityStr = $state(source.quantity_value !== null && source.quantity_scale !== null
    ? formatLedgerAmount(source.quantity_value, source.quantity_scale) : '');
  let carriedBasisStr = $state(source.carried_basis_value !== null && source.carried_basis_scale !== null
    ? formatLedgerAmount(source.carried_basis_value, source.carried_basis_scale) : '');
  let originalAcquiredOn = $state(source.original_acquired_on ?? '');
  let memo = $state(source.memo);
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let reasonInputElement: HTMLInputElement | undefined = $state();

  $effect(() => {
    reasonInputElement?.focus();
  });

  let review = $state<{
    impacts: ReconciliationImpactResponse['affected_checkpoints'];
    gainImpact: GainImpact | null;
    gainRefreshed: boolean;
    payload: ExternalTransferInRequest;
  } | null>(null);

  const holdingAccounts = $derived((accountsQuery.data?.accounts ?? []).filter((account: AccountResponse) =>
    (account.account_kind === 'security_holding' || account.account_kind === 'fund_holding') &&
    (account.status === 'active' || account.id === source.destination_account_id)));
  const currenciesByID = $derived(new Map<number, CurrencyResponse>(
    (currenciesQuery.data?.currencies ?? []).map((currency: CurrencyResponse) => [currency.id, currency])));
  const basisCurrency = $derived(currenciesByID.get(source.cost_commodity_id));
  const gainRows = $derived(review?.gainImpact
    ? gainImpactRows(review.gainImpact.changes, gainImpactCurrency(currenciesByID), locale) : []);
  const canSubmit = $derived(!!reason.trim() && effectiveOn !== '' && holdingAccountID !== '' &&
    !!quantityStr.trim() && !!carriedBasisStr.trim());

  function buildPayload(): ExternalTransferInRequest | null {
    const amounts = parseTransferInAmounts({ quantityStr, carriedBasisStr });
    if (!amounts.ok) {
      formError = new TranslatedFormError(amounts.reason === 'too_large'
        ? m.investments_form_amount_too_large()
        : amounts.field === 'quantity'
          ? m.investments_transfer_quantity_error()
          : m.investments_transfer_basis_error());
      return null;
    }
    if (originalAcquiredOn && originalAcquiredOn > effectiveOn) {
      formError = new TranslatedFormError(m.investments_transfer_original_date_error());
      return null;
    }
    return {
      effective_on: effectiveOn, holding_account_id: Number(holdingAccountID), commodity_id: source.commodity_id,
      quantity_value: amounts.values.quantity.value, quantity_scale: amounts.values.quantity.scale,
      carried_basis_value: amounts.values.carriedBasis.value, carried_basis_scale: amounts.values.carriedBasis.scale,
      cost_commodity_id: source.cost_commodity_id, original_acquired_on: originalAcquiredOn || undefined,
      source_evidence: source.source_evidence, memo: memo.trim() || undefined
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

  // Preview through the actual replacement writer; checkpoints or changed
  // gains go to the confirmation.
  async function reviewImpact(payload: ExternalTransferInRequest, refreshed: boolean): Promise<boolean> {
    const impact = await previewTransferInReplacement(transactionID, { reason: reason.trim(), replacement: payload });
    if (!impactNeedsReview(impact)) return false;
    const gainImpact = hasGainChanges(impact.gain_impact) ? impact.gain_impact : null;
    review = { impacts: impact.affected_checkpoints, gainImpact, gainRefreshed: refreshed && !!gainImpact, payload };
    return true;
  }

  async function commit(payload: ExternalTransferInRequest, override: boolean, acknowledgement: string) {
    await replaceTransferIn(transactionID, {
      reason: reason.trim(), replacement: payload,
      ...(override ? { reconciliation_override: true } : {}),
      ...(acknowledgement ? { gain_impact_acknowledgement: acknowledgement } : {})
    }, csrfToken);
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

  // The gain set changed since review: show the current set, not a dead end.
  async function recover(error: unknown, payload: ExternalTransferInRequest) {
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
  <h2 class="text-base font-semibold text-foreground">{m.transactions_investment_replace_transfer_title()}</h2>
  <p class="text-sm text-muted">{m.transactions_investment_replace_transfer_in_copy()}</p>

  <div>
    <label for="transfer-in-correction-reason" class="mb-1 block text-sm font-medium text-foreground">
      {m.transactions_investment_replace_reason()}
    </label>
    <input id="transfer-in-correction-reason" type="text" bind:this={reasonInputElement} bind:value={reason}
      maxlength="500" required
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
    <div>
      <label for="transfer-in-correction-date" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_transfer_effective_date()}
      </label>
      <input id="transfer-in-correction-date" type="date" bind:value={effectiveOn} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
    </div>
    <div>
      <label for="transfer-in-correction-holding" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_form_holding_account()}
      </label>
      <select id="transfer-in-correction-holding" bind:value={holdingAccountID} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {#each holdingAccounts as account (account.id)}
          <option value={String(account.id)}>{account.name}</option>
        {/each}
      </select>
    </div>
  </div>

  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
    <div>
      <label for="transfer-in-correction-quantity" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_form_quantity()}
      </label>
      <input id="transfer-in-correction-quantity" type="text" inputmode="decimal" bind:value={quantityStr} required
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground" />
    </div>
    <div>
      <label for="transfer-in-correction-basis" class="mb-1 block text-sm font-medium text-foreground">
        {m.investments_transfer_carried_basis()} {basisCurrency?.code ?? ''}
      </label>
      <input id="transfer-in-correction-basis" type="text" inputmode="decimal" bind:value={carriedBasisStr} required
        aria-describedby="transfer-in-correction-basis-help"
        class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm text-foreground" />
      <p id="transfer-in-correction-basis-help" class="mt-1 text-xs text-muted">{m.investments_transfer_basis_help()}</p>
    </div>
  </div>

  <div>
    <label for="transfer-in-correction-original" class="mb-1 block text-sm font-medium text-foreground">
      {m.investments_transfer_original_date()}
    </label>
    <input id="transfer-in-correction-original" type="date" bind:value={originalAcquiredOn}
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <div>
    <label for="transfer-in-correction-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label>
    <input id="transfer-in-correction-memo" type="text" bind:value={memo} maxlength="500"
      class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" />
  </div>

  <APIFormError error={formError} id="transfer-in-correction-error" />

  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel} disabled={pending}
      class="min-h-10 rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground hover:bg-control-hover">
      {m.investments_form_cancel()}
    </button>
    <button type="submit" disabled={!canSubmit || pending}
      class="min-h-10 rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50">
      {m.transactions_investment_replace_submit()}
    </button>
  </div>
</form>
