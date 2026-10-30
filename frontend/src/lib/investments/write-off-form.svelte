<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { tick, untrack } from 'svelte';
  import { parseISO } from 'date-fns';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions } from '#lib/api/currencies.ts';
  import {
    datedHoldingsQueryOptions, investmentInstrumentsQueryOptions,
    previewWriteOff, writeOffReconciliationImpact, recordWriteOff,
    type CostBasisMethod, type InvestmentWriteOffRequest, type SellPreviewResponse,
    type ReconciliationImpactResponse
  } from '#lib/api/investments.ts';
  import { parseMagnitude } from '#lib/investments/form-amounts.ts';
  import { COST_BASIS_METHODS, costBasisMethodLabel } from '#lib/investments/investment-labels.ts';
  import { parseCorrectionLotChoices } from '#lib/investments/trade-economics.ts';
  import { invalidateInvestmentReads } from '#lib/investments/invalidate.ts';
  import {
    gainAcknowledgement,
    gainImpactCurrency,
    gainImpactRows,
    hasGainChanges,
    impactNeedsReview,
    isStaleImpactRefusal,
    checkpointOverride,
    type ReviewedCheckpoints
  } from '#lib/investments/gain-impact.ts';
  import ReconciliationConfirm from '#lib/investments/reconciliation-confirm.svelte';
  import { coefficientSign, compareScaledAmounts } from '#lib/money/amount.ts';
  import { formatExactMoney, formatQuantity } from '#lib/money/format.ts';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { usePrefilledPosition } from '#lib/investments/investment-prefill.svelte.ts';
  import type { InvestmentPrefill } from '#lib/investments/investment-prefill.ts';

  let { csrfToken, onSaved, onCancel, prefill }: {
    csrfToken: string; onSaved: () => void; onCancel: () => void; prefill?: InvestmentPrefill;
  } = $props();

  // Pre-select the position the screen names (#186).
  usePrefilledPosition(untrack(() => prefill), () => holdings,
    (position) => `${position.account_id}:${position.commodity_id}:${position.cost_commodity_id}`, () => holdingKey, (key) => { holdingKey = key; });

  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());
  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  // Require a deliberate calendar date; discovery and the writer use that slot.
  let transactionDate = $state('');
  const holdingsQuery = createQuery(() => datedHoldingsQueryOptions(transactionDate));
  let holdingKey = $state('');
  let quantity = $state('');
  let method = $state<CostBasisMethod | undefined>();
  let quantities = $state<Record<number, string>>({});
  let reason = $state('');
  let pending = $state(false);
  let formError = $state<unknown>();
  let preview = $state<{
    result: SellPreviewResponse; impact: ReconciliationImpactResponse; payload: InvestmentWriteOffRequest;
  } | null>(null);
  let reviewing = $state(false);
  let gainRefreshed = $state(false);
  let recordButton = $state<HTMLButtonElement>();

  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' }));
  const accounts = $derived((accountsQuery.data?.accounts ?? []).filter(account => account.status === 'active' &&
    account.allows_postings && (account.account_kind === 'security_holding' || account.account_kind === 'fund_holding')));
  const instruments = $derived(instrumentsQuery.data?.instruments ?? []);
  const currenciesByID = $derived(new Map((currenciesQuery.data?.currencies ?? []).map(currency => [currency.id, currency])));
  const holdings = $derived((holdingsQuery.data ?? []).filter(holding => holding.position_side === 'long' &&
    coefficientSign(holding.quantity_value) > 0 && accounts.some(account => account.id === holding.account_id)));
  const holding = $derived(holdings.find(item => `${item.account_id}:${item.commodity_id}:${item.cost_commodity_id}` === holdingKey));
  const lots = $derived(holding?.lots ?? []);
  const instrument = $derived(instruments.find(item => item.commodity_id === holding?.commodity_id));
  const loading = $derived(accountsQuery.isPending || instrumentsQuery.isPending || currenciesQuery.isPending ||
    (!!transactionDate && holdingsQuery.isFetching));
  const loadError = $derived(accountsQuery.error ?? instrumentsQuery.error ?? currenciesQuery.error ??
    (transactionDate ? holdingsQuery.error : undefined));
  const canPreview = $derived(!loading && !loadError && !!transactionDate && !!holding && !!instrument &&
    !!quantity.trim() && !!reason.trim() && (method !== 'specific_lot' || lots.length > 0));
  const basisCurrency = $derived(currenciesByID.get(preview?.result.disposal_decision.cost_commodity_id ?? holding?.cost_commodity_id ?? 0));
  const gainImpact = $derived(preview && hasGainChanges(preview.impact.gain_impact) ? preview.impact.gain_impact : null);
  const gainRows = $derived(gainImpact ? gainImpactRows(gainImpact.changes, gainImpactCurrency(currenciesByID), locale) : []);


  function close() {
    if (pending) return;
    onCancel();
  }
  function cancelReview() {
    reviewing = false;
    void tick().then(() => recordButton?.focus());
  }
  function clearPreview() { preview = null; formError = undefined; }
  function changeDate() { holdingKey = ''; quantity = ''; quantities = {}; clearPreview(); }
  function changeHolding() { quantity = ''; quantities = {}; clearPreview(); }
  function money(value: string, scale: number): string {
    return formatExactMoney(value, scale, basisCurrency?.standard_scale ?? 2, locale);
  }

  function buildPayload(): InvestmentWriteOffRequest | null {
    if (!holding || !instrument) return null;
    const parsed = parseMagnitude(quantity, { maxScale: instrument.quantity_scale });
    if (!parsed.ok || coefficientSign(parsed.field.value) <= 0) {
      formError = new TranslatedFormError(m.investments_write_off_quantity_error());
      return null;
    }
    if (compareScaledAmounts(parsed.field, { value: holding.quantity_value, scale: holding.quantity_scale }) > 0) {
      formError = new TranslatedFormError(m.investments_write_off_exceeds());
      return null;
    }
    const choices = method === 'specific_lot' ? parseCorrectionLotChoices(
      lots.filter(lot => quantities[lot.lot_id]?.trim()).map(lot => ({ lotID: String(lot.lot_id), quantity: quantities[lot.lot_id] })),
      lots, parsed.field) : null;
    if (choices && !choices.ok) {
      formError = new TranslatedFormError(choices.reason === 'exceeds_available'
        ? m.investments_write_off_exceeds()
        : choices.reason === 'mismatch' ? m.investments_write_off_lot_mismatch()
          : m.transactions_investment_replace_lot_invalid());
      return null;
    }
    return {
      transaction_date: transactionDate, holding_account_id: holding.account_id, commodity_id: holding.commodity_id,
      cost_commodity_id: holding.cost_commodity_id,
      quantity_value: parsed.field.value, quantity_scale: parsed.field.scale, reason: reason.trim(),
      cost_basis_method: method, lot_allocations: choices?.ok ? choices.allocations : undefined
    };
  }

  async function runPreview(payload: InvestmentWriteOffRequest) {
    // Both endpoints simulate the writer and roll it back. No preview can
    // promise a loss while skipping a dependency or checkpoint refusal.
    const result = await previewWriteOff(payload);
    const impact = await writeOffReconciliationImpact(payload);
    preview = { result, impact, payload };
  }
  async function handlePreview(event: SubmitEvent) {
    event.preventDefault();
    if (!canPreview || pending) return;
    formError = undefined;
    const payload = buildPayload();
    if (!payload) return;
    pending = true;
    try { await runPreview(payload); }
    catch (error) { preview = null; formError = error; }
    finally { pending = false; }
  }
  async function commit(reviewed: ReviewedCheckpoints) {
    if (!preview || pending) return;
    pending = true;
    formError = undefined;
    const payload = preview.payload;
    try {
      await recordWriteOff({ ...payload,
        ...checkpointOverride(reviewed),
        gain_impact_acknowledgement: gainAcknowledgement(gainImpact) || undefined
      }, csrfToken);
      await invalidateInvestmentReads(queryClient);
      onSaved();
    } catch (error) {
      if (isStaleImpactRefusal(error)) {
        try {
          await runPreview(payload);
          reviewing = !!preview && impactNeedsReview(preview.impact);
          gainRefreshed = reviewing && !!gainImpact;
        } catch (previewError) { preview = null; formError = previewError; }
      } else { preview = null; formError = error; }
    } finally { pending = false; }
  }
  function record() {
    if (!preview || pending) return;
    if (impactNeedsReview(preview.impact)) { gainRefreshed = false; reviewing = true; }
    else void commit([]);
  }
  function confirm() {
    if (!preview || pending) return;
    reviewing = false;
    void commit(preview.impact.affected_checkpoints);
  }
</script>

<ModalDialog labelledby="write-off-title" {pending} onCancel={close} class="max-w-lg p-6">
  {#if reviewing && preview}
    <ReconciliationConfirm impacts={preview.impact.affected_checkpoints} {gainRows} {gainRefreshed} {pending}
      onCancel={cancelReview} onConfirm={confirm} />
  {/if}
  <form onsubmit={handlePreview} class="space-y-4" aria-busy={pending}>
    <h2 id="write-off-title" class="text-base font-semibold">{m.investments_write_off_title()}</h2>
    <p class="text-sm text-muted">{m.investments_write_off_help()}</p>
    <fieldset disabled={pending || reviewing} class="space-y-4">
      <div>
        <label for="write-off-date" class="mb-1 block text-sm font-medium">{m.investments_form_date()}</label>
        <input id="write-off-date" type="date" bind:value={transactionDate} required oninput={changeDate}
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm" />
      </div>
      {#if loading}
        <p class="text-sm text-muted" role="status">{m.investments_loading()}</p>
      {:else if loadError}
        <APIFormError error={loadError} />
      {:else if !transactionDate}
        <p class="text-sm text-muted" role="status">{m.investments_write_off_choose_date()}</p>
      {:else if holdings.length === 0}
        <p class="text-sm text-muted" role="status">{m.investments_write_off_empty()}</p>
      {:else}
        <div>
          <label for="write-off-holding" class="mb-1 block text-sm font-medium">{m.investments_form_holding_account()}</label>
          <select id="write-off-holding" bind:value={holdingKey} required onchange={changeHolding} aria-describedby="write-off-dated-hint"
            class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm">
            <option value="">{m.investments_form_select_account()}</option>
            {#each holdings as item (`${item.account_id}:${item.commodity_id}:${item.cost_commodity_id}`)}
              <option value={`${item.account_id}:${item.commodity_id}:${item.cost_commodity_id}`}>
                {accounts.find(account => account.id === item.account_id)?.name} · {instruments.find(inst => inst.commodity_id === item.commodity_id)?.display_name ?? `#${item.commodity_id}`} · {currenciesByID.get(item.cost_commodity_id)?.code}
              </option>
            {/each}
          </select>
          <p id="write-off-dated-hint" class="mt-1 text-xs text-muted">{m.investments_dated_holdings_hint()}</p>
        </div>
        {#if holding}
          <div>
            <label for="write-off-quantity" class="mb-1 block text-sm font-medium">{m.investments_form_quantity()}</label>
            <input id="write-off-quantity" type="text" inputmode="decimal" bind:value={quantity} required oninput={clearPreview}
              aria-describedby="write-off-available" class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm" />
            <p id="write-off-available" class="mt-1 text-xs text-muted">{m.investments_write_off_available({quantity: formatQuantity(holding.quantity_value, holding.quantity_scale, locale)})}</p>
          </div>
          <div>
            <label for="write-off-method" class="mb-1 block text-sm font-medium">{m.investments_form_cost_basis_method()}</label>
            <select id="write-off-method" bind:value={method} onchange={() => { quantities = {}; clearPreview(); }}
              class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm">
              <option value={undefined}>{m.investments_cash_in_lieu_default_method()}</option>
              {#each COST_BASIS_METHODS as choice (choice)}<option value={choice}>{costBasisMethodLabel(choice)}</option>{/each}
            </select>
          </div>
          {#if method === 'specific_lot'}
            <fieldset class="space-y-3 rounded-(--radius-control) border border-border p-3">
              <legend class="px-1 text-sm font-medium">{m.transactions_investment_replace_lots_title()}</legend>
              <p class="text-xs text-muted">{m.investments_write_off_lots_help()}</p>
              {#each lots as lot (lot.lot_id)}
                <div>
                  <label for={`write-off-lot-${lot.lot_id}`} class="mb-1 block text-sm">
                    {m.investments_write_off_lot({lot: String(lot.lot_id), date: dateFormatter.format(parseISO(lot.opened_on)), quantity: formatQuantity(lot.quantity_value, lot.quantity_scale, locale)})}
                  </label>
                  <input id={`write-off-lot-${lot.lot_id}`} type="text" inputmode="decimal" bind:value={quantities[lot.lot_id]} oninput={clearPreview}
                    class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 font-mono text-sm" />
                </div>
              {/each}
            </fieldset>
          {/if}
        {/if}
      {/if}
      <div>
        <label for="write-off-reason" class="mb-1 block text-sm font-medium">{m.investments_form_write_off_reason()}</label>
        <input id="write-off-reason" type="text" bind:value={reason} required maxlength="500" oninput={clearPreview}
          class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm" />
      </div>
      {#if preview}
        <section aria-labelledby="write-off-preview-title" aria-live="polite" class="space-y-2 rounded-(--radius-control) border border-border p-3">
          <h3 id="write-off-preview-title" class="text-sm font-semibold">{m.investments_write_off_preview_title()}</h3>
          {#if preview.result.realized_gain !== null && preview.result.realized_gain_scale !== null}
            <p class="text-sm font-semibold">{m.investments_write_off_result({result: money(preview.result.realized_gain, preview.result.realized_gain_scale), currency: basisCurrency?.code ?? ''})}</p>
          {:else}
            <p class="text-sm font-semibold">{m.investments_gain_unresolved()}</p>
            <p class="text-xs text-muted">{m.investments_write_off_unresolved_help()}</p>
          {/if}
          <p class="text-xs text-muted">{m.investments_sell_preview_method()}: {costBasisMethodLabel(preview.result.cost_basis_method)}</p>
          <ul class="space-y-1 text-sm">
            {#each preview.result.allocations as allocation (allocation.lot_id)}
              <li>{m.investments_cash_in_lieu_allocation({lot: String(allocation.lot_id), quantity: formatQuantity(allocation.quantity_value, allocation.quantity_scale, locale),
                basis: allocation.cost_basis_value !== null && allocation.cost_basis_scale !== null ? money(allocation.cost_basis_value, allocation.cost_basis_scale) : m.investments_basis_unknown(), currency: basisCurrency?.code ?? ''})}</li>
            {/each}
          </ul>
        </section>
      {/if}
    </fieldset>
    <APIFormError error={formError} id="write-off-error" />
    <div class="flex flex-wrap justify-end gap-3">
      <button type="button" onclick={close} disabled={pending || reviewing}
        class="min-h-10 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold">{m.investments_form_cancel()}</button>
      {#if preview}
        <button type="button" bind:this={recordButton} onclick={record} disabled={pending || reviewing || !csrfToken}
          class="min-h-10 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background disabled:opacity-50">
          {pending ? m.investments_transfer_pending() : m.investments_record_write_off()}
        </button>
      {:else}
        <button type="submit" disabled={!canPreview || pending || reviewing}
          class="min-h-10 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background disabled:opacity-50">
          {pending ? m.investments_transfer_internal_previewing() : m.investments_write_off_preview()}
        </button>
      {/if}
    </div>
  </form>
</ModalDialog>
