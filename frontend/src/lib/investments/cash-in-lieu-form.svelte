<script lang="ts">
  import { untrack } from 'svelte';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions } from '#lib/api/currencies.ts';
  import { cashInLieuLotsQueryKey, getCashInLieuLots, previewCashInLieu, recordCashInLieu, previewCashInLieuReplacement, replaceCashInLieu,
    type CashInLieuRequest, type CashInLieuPreviewResponse, type InvestmentTradeCorrectionContextResponse } from '#lib/api/investments.ts';
  import { parseMagnitude, parseMoneyMagnitude } from './form-amounts';
  import { parseCorrectionLotChoices } from './trade-economics';
  import { compareScaledAmounts, formatLedgerAmount } from '#lib/money/amount.ts';
  import { formatExactMoney, formatQuantity } from '#lib/money/format.ts';
  import { invalidateInvestmentReads } from './invalidate';
  import { gainAcknowledgement, gainImpactCurrency, gainImpactRows, hasGainChanges, impactNeedsReview, isGainAcknowledgementRefusal } from './gain-impact';
  import ReconciliationConfirm from './reconciliation-confirm.svelte';
  import { TranslatedFormError } from '#lib/form-errors.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  let { csrfToken, onSaved, onCancel, split, correction }: {
    csrfToken: string; onSaved: () => void; onCancel: () => void;
    split?: { transactionID: number; holdingAccountID: number; commodityID: number; effectiveOn: string };
    correction?: InvestmentTradeCorrectionContextResponse;
  } = $props();
  const initial = untrack(() => correction);
  const initialSplit = untrack(() => split);
  const splitID = initial?.split_transaction_id ?? initialSplit?.transactionID ?? 0;
  const holdingID = initial?.holding_account_id ?? initialSplit?.holdingAccountID ?? 0;
  const queryClient = useQueryClient();
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  let disposalOn = $state(initial?.event_date ?? initialSplit?.effectiveOn ?? '');
  let paymentOn = $state(initial?.settlement_date ?? initialSplit?.effectiveOn ?? '');
  let cashAccountID = $state(initial ? String(initial.cash_account_id) : '');
  let currencyID = $state(initial ? String(initial.cost_commodity_id) : '');
  let quantity = $state(initial ? formatLedgerAmount(initial.quantity_value, initial.quantity_scale) : '');
  let proceeds = $state(initial ? formatLedgerAmount(initial.net_value, initial.net_scale) : '');
  let method = $state<CashInLieuRequest['cost_basis_method']>(initial?.cost_basis_method as CashInLieuRequest['cost_basis_method'] ?? undefined);
  let memo = $state(initial?.memo ?? '');
  let reason = $state('');
  let quantities = $state<Record<number,string>>(Object.fromEntries((initial?.effective_elected_lots ?? []).map(e => [e.lot_id,formatLedgerAmount(e.quantity_value,e.quantity_scale)])));
  let pending = $state(false);
  let formError = $state<unknown>();
  let firstInput = $state<HTMLInputElement>();
  let didFocus = false;
  let preview = $state<{ result: CashInLieuPreviewResponse; payload: CashInLieuRequest } | null>(null);
  let reviewing = $state(false);
  let gainRefreshed = $state(false);
  const locale = $derived(getLocale());
  const accounts = $derived(accountsQuery.data?.accounts ?? []);
  const cashAccounts = $derived(accounts.filter(a => a.account_class === 'asset' && a.allows_postings &&
    (a.status === 'active' || a.id === initial?.cash_account_id) && a.account_kind !== 'security_holding' && a.account_kind !== 'fund_holding'));
  const currencies = $derived(currenciesQuery.data?.currencies ?? []);
  const currency = $derived(currencies.find(c => c.id === Number(currencyID)));
  const currencyMap = $derived(new Map(currencies.map(c => [c.id,c])));
  const loading = $derived(accountsQuery.isPending || currenciesQuery.isPending);
  const loadError = $derived(accountsQuery.isError || currenciesQuery.isError);
  const lotsQuery = createQuery(() => ({
    queryKey: [...cashInLieuLotsQueryKey,splitID,disposalOn,currencyID,initial?.transaction_id],
    queryFn: () => getCashInLieuLots(splitID,disposalOn,Number(currencyID),initial?.transaction_id),
    enabled: method === 'specific_lot' && !!disposalOn && !!currencyID && splitID > 0
  }));
  const availableLots = $derived(lotsQuery.data?.lots ?? []);
  const canPreview = $derived(!loading && !loadError && splitID > 0 && !!disposalOn && !!paymentOn && !!cashAccountID && !!currencyID && !!quantity.trim() && !!proceeds.trim() &&
    (!initial || !!reason.trim()) && (method !== 'specific_lot' || (!lotsQuery.isPending && !lotsQuery.isError && availableLots.length > 0)));
  const gainImpact = $derived(preview && hasGainChanges(preview.result.impact.gain_impact) ? preview.result.impact.gain_impact : null);
  const gainRows = $derived(gainImpact ? gainImpactRows(gainImpact.changes,gainImpactCurrency(currencyMap),locale) : []);
  $effect(() => { if (!loading && firstInput && !didFocus) {firstInput.focus(); didFocus=true;} });

  function costBasisMethodLabel(value:string) {
    if (value==='fifo') return m.transactions_investment_replace_method_fifo();
    if (value==='lifo') return m.transactions_investment_replace_method_lifo();
    if (value==='average_cost') return m.transactions_investment_replace_method_average();
    return m.transactions_investment_replace_method_specific();
  }
  function clearPreview() {preview=null;formError=undefined;}
  function selectCash() {
    const account=accounts.find(a => a.id===Number(cashAccountID));
    if (!currencyID && account?.default_commodity_id && currencies.some(c => c.id===account.default_commodity_id)) currencyID=String(account.default_commodity_id);
    clearPreview();
  }
  function money(value:string,scale:number) { return formatExactMoney(value,scale,currency?.standard_scale ?? 2,locale); }
  function buildPayload(): CashInLieuRequest | null {
    const q=parseMagnitude(quantity,{maxScale:24});
    if (!q.ok || q.field.value==='0' || compareScaledAmounts(q.field,{value:'1',scale:0})>=0) {
      formError=new TranslatedFormError(m.investments_cash_in_lieu_fraction_error());return null;
    }
    const p=parseMoneyMagnitude(proceeds,{maxScale:Math.max(currency?.standard_scale ?? 2,initial?.net_scale ?? 0)});
    if (!p.ok || p.field.value==='0') {formError=new TranslatedFormError(m.investments_capital_return_amount_error());return null;}
    if (paymentOn<disposalOn) {formError=new TranslatedFormError(m.investments_cash_in_lieu_dates_error());return null;}
    const choices=method==='specific_lot' ? parseCorrectionLotChoices(Object.entries(quantities).filter(([lot,q])=>q?.trim() && availableLots.some(a=>String(a.lot_id)===lot)).map(([lot,q])=>({lotID:lot,quantity:q})),availableLots,q.field):null;
    if (choices && !choices.ok) {formError=new TranslatedFormError(m.transactions_investment_replace_lot_mismatch());return null;}
    return {split_transaction_id:splitID,disposal_on:disposalOn,payment_on:paymentOn,cash_account_id:Number(cashAccountID),currency_id:Number(currencyID),quantity_value:q.field.value,quantity_scale:q.field.scale,
      proceeds_value:p.field.value,proceeds_scale:p.field.scale,cost_basis_method:method,lot_allocations:choices?.ok ? choices.allocations:undefined,memo:memo.trim() || undefined};
  }
  async function runPreview(payload:CashInLieuRequest) {
    const result=initial ? await previewCashInLieuReplacement(initial.transaction_id,{reason:reason.trim(),replacement:payload}) : await previewCashInLieu(payload);
    preview={result,payload};
  }
  async function handlePreview(event:SubmitEvent) {
    event.preventDefault(); if (!canPreview) return;
    formError=undefined;const payload=buildPayload();if (!payload) return;
    pending=true;
    try {await runPreview(payload);} catch(error){preview=null;formError=error;} finally{pending=false;}
  }
  async function commit(override:boolean) {
    if (!preview) return;pending=true;formError=undefined;
    try {
      const acknowledgement=gainAcknowledgement(gainImpact);
      const guards={reconciliation_override:override,gain_impact_acknowledgement:acknowledgement || undefined};
      if (initial) await replaceCashInLieu(initial.transaction_id,{reason:reason.trim(),replacement:preview.payload,...guards},csrfToken);
      else await recordCashInLieu({...preview.payload,...guards},csrfToken);
      await invalidateInvestmentReads(queryClient);onSaved();
    } catch(error) {
      if (isGainAcknowledgementRefusal(error) && preview) {
        try {await runPreview(preview.payload);reviewing=!!preview && impactNeedsReview(preview.result.impact);gainRefreshed=reviewing;}
        catch(refreshError){preview=null;formError=refreshError;}
      } else formError=error;
    } finally{pending=false;}
  }
  function record() {
    if (!preview) return;
    if (impactNeedsReview(preview.result.impact)) {gainRefreshed=false;reviewing=true;} else void commit(false);
  }
  function confirm() {
    if (!preview) return;const override=preview.result.impact.affected_checkpoints.length>0;reviewing=false;void commit(override);
  }
</script>

{#if reviewing && preview}
  <ReconciliationConfirm impacts={preview.result.impact.affected_checkpoints} {gainRows} {gainRefreshed} {pending} onCancel={()=>reviewing=false} onConfirm={confirm} />
{/if}
<form onsubmit={handlePreview} class="space-y-4" aria-busy={pending}>
  <h2 class="text-base font-semibold text-foreground">{initial ? m.investments_cash_in_lieu_correct() : m.investments_cash_in_lieu_title()}</h2>
  <p class="text-sm text-muted">{m.investments_cash_in_lieu_help()}</p>
  <p class="text-sm text-foreground">{accounts.find(a=>a.id===holdingID)?.name} · <a class="text-accent underline" href={`/app/transactions?transaction_id=${splitID}`}>{m.investments_cash_in_lieu_split({id:String(splitID)})}</a></p>
  {#if loading}<p class="text-sm text-muted" role="status">{m.investments_loading()}</p>
  {:else if loadError}<p class="text-sm text-danger" role="alert">{m.investments_transfer_load_error()}</p>
  {:else}
    <fieldset disabled={pending} class="space-y-4">
      {#if initial}<div><label for="cil-reason" class="mb-1 block text-sm font-medium text-foreground">{m.transactions_investment_replace_reason()}</label>
        <input id="cil-reason" type="text" bind:value={reason} required maxlength="500" oninput={clearPreview} class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" /></div>{/if}
      <div><label for="cil-quantity" class="mb-1 block text-sm font-medium text-foreground">{m.investments_cash_in_lieu_quantity()}</label>
        <input id="cil-quantity" bind:this={firstInput} type="text" inputmode="decimal" bind:value={quantity} required oninput={clearPreview} class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" /></div>
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <div><label for="cil-disposal" class="mb-1 block text-sm font-medium text-foreground">{m.investments_cash_in_lieu_disposal()}</label><input id="cil-disposal" type="date" bind:value={disposalOn} required oninput={clearPreview} class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" /></div>
        <div><label for="cil-payment" class="mb-1 block text-sm font-medium text-foreground">{m.investments_capital_return_payment_date()}</label><input id="cil-payment" type="date" bind:value={paymentOn} required oninput={clearPreview} class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" /></div>
      </div>
      <div><label for="cil-cash" class="mb-1 block text-sm font-medium text-foreground">{m.investments_capital_return_cash_account()}</label><select id="cil-cash" bind:value={cashAccountID} required onchange={selectCash} class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"><option value="">{m.investments_form_select_account()}</option>{#each cashAccounts as a(a.id)}<option value={String(a.id)}>{a.name}</option>{/each}</select></div>
      <div><label for="cil-currency" class="mb-1 block text-sm font-medium text-foreground">{m.investments_trade_settlement_currency()}</label><select id="cil-currency" bind:value={currencyID} required onchange={clearPreview} class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"><option value="">{m.accounts_field_choose_currency()}</option>{#each currencies as c(c.id)}<option value={String(c.id)}>{c.code}</option>{/each}</select></div>
      <div><label for="cil-proceeds" class="mb-1 block text-sm font-medium text-foreground">{m.investments_cash_in_lieu_proceeds()}</label><input id="cil-proceeds" type="text" inputmode="decimal" bind:value={proceeds} required oninput={clearPreview} class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" /></div>
      <div><label for="cil-method" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_cost_basis_method()}</label><select id="cil-method" bind:value={method} onchange={clearPreview} class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"><option value={undefined}>{m.investments_cash_in_lieu_default_method()}</option>{#each ['fifo','lifo','average_cost','specific_lot'] as choice}<option value={choice}>{costBasisMethodLabel(choice)}</option>{/each}</select></div>
      {#if method==='specific_lot'}
        {#if lotsQuery.isPending}<p role="status" class="text-sm text-muted">{m.investments_loading()}</p>
        {:else if lotsQuery.isError}<APIFormError error={lotsQuery.error} />
        {:else if availableLots.length===0}<p role="status" class="text-sm text-muted">{m.investments_cash_in_lieu_no_lots()}</p>
        {:else}{#each availableLots as lot(lot.lot_id)}<div><label for={`cil-lot-${lot.lot_id}`} class="mb-1 block text-sm text-foreground">{m.investments_cash_in_lieu_lot_quantity({lot:String(lot.lot_id),quantity:formatQuantity(lot.quantity_value,lot.quantity_scale,locale)})}</label><input id={`cil-lot-${lot.lot_id}`} type="text" inputmode="decimal" bind:value={quantities[lot.lot_id]} oninput={clearPreview} class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" /></div>{/each}{/if}
      {/if}
      <div><label for="cil-memo" class="mb-1 block text-sm font-medium text-foreground">{m.investments_form_memo()}</label><input id="cil-memo" type="text" bind:value={memo} maxlength="500" oninput={clearPreview} class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground" /></div>
      {#if preview}
        <section aria-labelledby="cil-preview-title" aria-live="polite" class="space-y-2 rounded-(--radius-control) border border-border p-3">
          <h3 id="cil-preview-title" class="text-sm font-semibold text-foreground">{m.investments_cash_in_lieu_preview_title()}</h3>
          <p class="text-sm text-foreground">{m.investments_cash_in_lieu_result({basis:money(preview.result.disposed_basis_value,preview.result.disposed_basis_scale),gain:money(preview.result.realized_gain_value,preview.result.realized_gain_scale),currency:currency?.code ?? '',method:costBasisMethodLabel(preview.result.cost_basis_method)})}</p>
          <ul class="space-y-1 text-sm text-foreground">{#each preview.result.allocations as a(a.lot_id)}<li>{m.investments_cash_in_lieu_allocation({lot:String(a.lot_id),quantity:formatQuantity(a.quantity_value,a.quantity_scale,locale),basis:money(a.cost_basis_value,a.cost_basis_scale),currency:currency?.code ?? ''})}</li>{/each}</ul>
        </section>
      {/if}
    </fieldset>
  {/if}
  <APIFormError error={formError} />
  <div class="flex flex-wrap justify-end gap-3">
    <button type="button" onclick={onCancel} disabled={pending} class="rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground">{m.investments_form_cancel()}</button>
    {#if preview}<button type="button" onclick={record} disabled={pending || !csrfToken} class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background disabled:opacity-50">{pending ? m.investments_transfer_pending() : initial ? m.investments_cash_in_lieu_correct_submit() : m.investments_cash_in_lieu_submit()}</button>
    {:else}<button type="submit" disabled={!canPreview || pending} class="rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background disabled:opacity-50">{pending ? m.investments_transfer_internal_previewing() : m.investments_cash_in_lieu_preview()}</button>{/if}
  </div>
</form>
