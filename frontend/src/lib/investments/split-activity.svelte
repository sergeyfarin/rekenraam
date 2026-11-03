<script lang="ts">
  import { createInfiniteQuery, createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import { page } from '$app/state';
  import { currenciesQueryOptions } from '#lib/api/currencies.ts';
  import { getSplitActivity, investmentSplitsQueryKey, previewSplitOperationReversal, reverseSplitOperation,
    type InvestmentSplitActivity, type ReconciliationImpactResponse } from '#lib/api/investments.ts';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import SplitForm from './split-form.svelte';
  import CashInLieuForm from './cash-in-lieu-form.svelte';
  import ReconciliationConfirm from './reconciliation-confirm.svelte';
  import { invalidateInvestmentReads } from './invalidate';
  import { checkpointOverride, gainAcknowledgement, gainImpactCurrency, gainImpactRows, impactNeedsReview, isStaleImpactRefusal } from './gain-impact';
  import { formatQuantity } from '#lib/money/format.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  let { csrfToken }: {csrfToken: string} = $props();
  const client = useQueryClient();
  const targetOperation = $derived.by(() => {
    const value=page.url.searchParams.get('split_operation_id');
    if(!value || !/^[1-9][0-9]*$/.test(value))return undefined;
    const id=Number(value);return Number.isSafeInteger(id) ? id : undefined;
  });
  const query = createInfiniteQuery(() => ({
    queryKey: [...investmentSplitsQueryKey,targetOperation], initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => getSplitActivity(pageParam,targetOperation), getNextPageParam: (last) => last.next_cursor ?? undefined
  }));
  const currencies = createQuery(() => currenciesQueryOptions());
  const rows = $derived(query.data?.pages.flatMap((p) => p.splits) ?? []);
  const locale = $derived(getLocale());
  const dateFormatter = $derived(new Intl.DateTimeFormat(locale,{year:'numeric',month:'short',day:'numeric'}));
  const numberFormatter = $derived(new Intl.NumberFormat(locale));
  const currencyByID = $derived(new Map((currencies.data?.currencies ?? []).map((currency) => [currency.id,currency])));
  let selected = $state<InvestmentSplitActivity | null>(null);
  let mode = $state<'replace' | 'reverse' | 'settle'>('replace');
  let reason = $state('');
  let pending = $state(false);
  let error = $state<unknown>();
  let impact = $state.raw<ReconciliationImpactResponse | null>(null);
  let reviewedReason = $state('');
  let gainRefreshed = $state(false);
  const gainRows = $derived(impact?.gain_impact ? gainImpactRows(impact.gain_impact.changes,gainImpactCurrency(currencyByID),locale) : []);
  const modalTitle = $derived(mode === 'replace' ? m.transactions_investment_replace_split_title() : mode === 'settle' ? m.investments_cash_in_lieu_title() : m.transactions_investment_reverse_split_action());

  const reference = (row: InvestmentSplitActivity): string => typeof row.source_evidence.reference === 'string' ? row.source_evidence.reference : '';
  const standing = (row: InvestmentSplitActivity) => row.standing === 'effective' ? m.investments_split_standing_effective() : row.standing === 'superseded' ? m.investments_split_standing_superseded() : m.investments_split_standing_reversed();
  function open(row: InvestmentSplitActivity, action: typeof mode) {
    selected=row; mode=action; reason=''; error=undefined; impact=null; gainRefreshed=false;
  }
  function close() { if (!pending) {selected=null;impact=null;} }
  async function saved() { selected=null;impact=null; }
  async function commitReversal() {
    if (!selected || !impact) return;
    pending=true;error=undefined;
    try {
      await reverseSplitOperation(selected.operation_id,{reason:reviewedReason,
        ...checkpointOverride(impact.affected_checkpoints),
        gain_impact_acknowledgement:gainAcknowledgement(impact.gain_impact)},csrfToken);
      await invalidateInvestmentReads(client);
      await saved();
    } catch (failure) {
      if (isStaleImpactRefusal(failure)) {
        try { impact=await previewSplitOperationReversal(selected.operation_id,{reason:reviewedReason});gainRefreshed=true; }
        catch (previewFailure) {impact=null;error=previewFailure;}
      } else {impact=null;error=failure;}
    } finally {pending=false;}
  }
  async function reverse(event: SubmitEvent) {
    event.preventDefault();if (!selected || !reason.trim() || pending || !csrfToken) return;
    pending=true;error=undefined;reviewedReason=reason.trim();
    try {
      impact=await previewSplitOperationReversal(selected.operation_id,{reason:reviewedReason});
      if (!impactNeedsReview(impact)) await commitReversal();
    } catch(failure) {impact=null;error=failure;}
    finally {pending=false;}
  }
</script>

<section class="space-y-4" aria-labelledby="split-activity-title">
  <h2 id="split-activity-title" class="text-base font-semibold text-foreground">{m.investments_split_history_title()}</h2>
  <p class="text-sm text-muted">{m.investments_split_history_help()}</p>
  {#if targetOperation}<a href="/app/investments?tab=splits" class="text-sm text-accent underline">{m.investments_split_show_all()}</a>{/if}
  {#if query.isPending}
    <p role="status" class="text-sm text-muted">{m.investments_loading()}</p>
  {:else if query.isError}
    <APIFormError error={query.error} />
  {:else if rows.length === 0}
    <p role="status" class="text-sm text-muted">{m.investments_split_history_empty()}</p>
  {:else}
    {#each rows as row (row.operation_id)}
      <article class="space-y-2 rounded-(--radius-panel) border border-border bg-surface p-4" aria-label={m.investments_split_record({id:String(row.operation_id)})}>
        <h3 class="font-medium text-foreground">{row.account_name} · {row.instrument_name}</h3>
        <p class="text-sm text-foreground">{dateFormatter.format(parseISO(row.effective_on))} · {numberFormatter.format(row.ratio_numerator)}:{numberFormatter.format(row.ratio_denominator)} · {standing(row)}</p>
        <p class="text-sm text-muted">{m.investments_split_total_change({delta:formatQuantity(row.quantity_delta_value,row.quantity_delta_scale,locale)})}</p>
        {#if reference(row)}<p class="text-sm text-foreground">{reference(row)}</p>{/if}
        {#if row.memo}<p class="text-sm text-foreground">{row.memo}</p>{/if}
        {#if row.reason}<p class="text-sm text-muted">{row.reason}</p>{/if}
        <details open={page.url.searchParams.get('split_operation_id') === String(row.operation_id)}>
          <summary class="cursor-pointer text-sm text-accent">{m.investments_split_source_details()}</summary>
          <pre class="overflow-x-auto whitespace-pre-wrap break-words text-xs text-muted">{JSON.stringify(row.source_evidence,null,2)}</pre>
          <time datetime={row.created_at} class="text-xs text-muted">{dateFormatter.format(parseISO(row.created_at))}</time>
        </details>
        {#if row.transaction_id}<a class="text-sm text-accent underline" href={`/app/ledger/transactions?transaction_id=${row.transaction_id}`}>{m.investments_split_open_journal()}</a>{/if}
        {#if row.correction_of_operation_id}<p class="text-xs text-muted">{m.investments_split_corrects({id:String(row.correction_of_operation_id)})}</p>{/if}
        {#if row.successor_operation_id}
          {#if row.standing === 'superseded'}
            <a class="text-xs text-accent underline" href={`/app/investments?tab=splits&split_operation_id=${row.successor_operation_id}`}>{m.investments_split_successor({id:String(row.successor_operation_id)})}</a>
          {:else}<p class="text-xs text-muted">{m.investments_split_successor({id:String(row.successor_operation_id)})}</p>{/if}
        {/if}
        {#if row.standing === 'effective'}
          {#if row.settled}<p class="text-sm text-muted">{m.investments_split_settled_hint()}</p>{/if}
          <div class="flex flex-wrap gap-2">
            <button type="button" disabled={!csrfToken} onclick={()=>open(row,'replace')} class="rounded-(--radius-control) border border-border px-3 py-2 text-sm text-foreground disabled:opacity-60">{m.transactions_investment_replace_split_action()}</button>
            <button type="button" disabled={!csrfToken} onclick={()=>open(row,'reverse')} class="rounded-(--radius-control) border border-border px-3 py-2 text-sm text-danger disabled:opacity-60">{m.transactions_investment_reverse_split_action()}</button>
          </div>
        {/if}
        <button type="button" disabled={!csrfToken} onclick={()=>open(row,'settle')} class="rounded-(--radius-control) border border-border px-3 py-2 text-sm text-foreground disabled:opacity-60">{m.investments_cash_in_lieu_title()}</button>
      </article>
    {/each}
    {#if query.hasNextPage}<button type="button" disabled={query.isFetchingNextPage} onclick={()=>query.fetchNextPage()} class="rounded-(--radius-control) border border-border px-4 py-2 text-sm text-foreground disabled:opacity-60">{m.transactions_load_more()}</button>{/if}
  {/if}
</section>

{#if selected}
  <ModalDialog labelledby="split-action-title" {pending} onCancel={close} class="max-w-lg p-6">
    <h2 id="split-action-title" class="mb-4 text-base font-semibold">{modalTitle}</h2>
    {#if mode === 'replace'}
      <SplitForm {csrfToken} correction={{operationID:selected.operation_id,terms:selected}} onSaved={saved} onCancel={close}/>
    {:else if mode === 'settle'}
      <CashInLieuForm {csrfToken} split={{operationID:selected.operation_id,holdingAccountID:selected.holding_account_id,commodityID:selected.commodity_id,effectiveOn:selected.effective_on,historical:selected.standing !== 'effective'}} onSaved={saved} onCancel={close}/>
    {:else}
      {#if impact && impactNeedsReview(impact)}
        <ReconciliationConfirm impacts={impact.affected_checkpoints} {gainRows} {gainRefreshed} {pending} onCancel={()=>impact=null} onConfirm={commitReversal}/>
      {/if}
      <form onsubmit={reverse} class="space-y-4" aria-busy={pending}>
        <label for="split-reversal-reason" class="block text-sm font-medium">{m.transactions_investment_reverse_reason()}</label>
        <input id="split-reversal-reason" bind:value={reason} oninput={()=>impact=null} required maxlength="500" disabled={pending} class="w-full rounded-(--radius-control) border border-border bg-control px-3 py-2"/>
        <APIFormError error={error}/>
        <div class="flex gap-2">
          <button type="submit" disabled={pending || !reason.trim() || !csrfToken} class="rounded-(--radius-control) bg-danger px-4 py-2 text-sm text-background disabled:opacity-60">{m.transactions_investment_reverse_confirm()}</button>
          <button type="button" disabled={pending} onclick={close} class="rounded-(--radius-control) border border-border px-4 py-2 text-sm">{m.investments_form_cancel()}</button>
        </div>
      </form>
    {/if}
  </ModalDialog>
{/if}
