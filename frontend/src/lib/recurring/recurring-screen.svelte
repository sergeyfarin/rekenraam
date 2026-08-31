<script lang="ts">
  import { createQuery, createInfiniteQuery, useQueryClient } from '@tanstack/svelte-query';
  import { addDays, formatISO, parseISO } from 'date-fns';
  import StatePanel from '$lib/components/state-panel.svelte';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import TransactionEditor from '$lib/transactions/transaction-editor.svelte';
  import TemplateEditor from './template-editor.svelte';
  import { m } from '$lib/paraglide/messages.js';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import { authSessionQueryOptions } from '$lib/api/auth';
  import { currenciesQueryOptions } from '$lib/api/currencies';
  import { getTransaction, getPostReconciliationImpact, postTransaction, deleteDraftTransaction, transactionsQueryKey, type TransactionResponse, type ReconciliationImpactResponse } from '$lib/api/transactions';
  import { recurringTemplatesQueryOptions, recurringDueInfiniteQueryOptions, recurringSummaryQueryOptions, getRecurringOccurrences, archiveRecurringTemplate, updateRecurringTemplate, skipRecurringOccurrence, retryRecurringOccurrence, runRecurringNow, type RecurringTemplate, type RecurringDueResponse } from '$lib/api/recurring';
  import { formatQuantity } from '$lib/money/format';
  import { displayDate, scheduleLabel } from './recurring-model';

  const session = createQuery(() => authSessionQueryOptions());
  const summary = createQuery(() => recurringSummaryQueryOptions());
  let includeArchived = $state(false);
  const templates = createQuery(() => recurringTemplatesQueryOptions(includeArchived));
  const due = createInfiniteQuery(() => ({ ...recurringDueInfiniteQueryOptions(), refetchInterval: 30_000 }));
  const currencies = createQuery(() => currenciesQueryOptions());
  const queryClient = useQueryClient();
  const locale = $derived(getLocale());
  const csrf = $derived(session.data?.csrf_token ?? '');
  type DueItem = RecurringDueResponse['items'][number];
  const items = $derived(due.data?.pages.flatMap(p => p.items) ?? []);
  let tab = $state<'due' | 'templates'>('due');
  let error = $state<unknown>();
  let notice = $state('');
  let pending = $state(false);
  let selected = $state<number[]>([]);
  let dialog: HTMLDialogElement;
  type Review = { kind: 'post'; items: DueItem[]; impacts: ReconciliationImpactResponse[] }
    | { kind: 'skip'; templateID: number; date: string }
    | { kind: 'discard'; transactionID: number }
    | { kind: 'archive'; template: RecurringTemplate }
    | { kind: 'template'; template?: RecurringTemplate }
    | { kind: 'edit'; transaction: TransactionResponse };
  let review = $state<Review | null>(null);
  let reason = $state('');
  let dialogError = $state<unknown>();
  let impactChanged = $state(false);
  let inspecting = $state<RecurringTemplate>();
  const occurrenceRange = $derived.by(() => {
    const from = summary.data?.today;
    return from ? { from, to: formatISO(addDays(parseISO(from), 366), { representation: 'date' }) } : undefined;
  });
  const occurrences = createQuery(() => ({
    queryKey: ['api', 'recurring', 'occurrences', inspecting?.id, occurrenceRange],
    queryFn: () => getRecurringOccurrences(inspecting!.id, occurrenceRange!.from, occurrenceRange!.to),
    enabled: !!inspecting && !!occurrenceRange
  }));
  $effect(() => {
    if (!dialog) return;
    if (review && !dialog.open) dialog.showModal();
    else if (!review && dialog.open) dialog.close();
  });
  async function refresh() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['api', 'recurring'] }),
      queryClient.invalidateQueries({ queryKey: transactionsQueryKey }),
      queryClient.invalidateQueries({ queryKey: ['api', 'accounts'] }),
      queryClient.invalidateQueries({ queryKey: ['api', 'reports'] })
    ]);
  }
  function open(next: Review) { impactChanged = false; dialogError = undefined; reason = ''; review = next; }
  function close() { if (!pending) review = null; }
  async function action(fn: () => Promise<void>) {
    if (pending) return;
    pending = true; error = undefined; notice = '';
    try { await fn(); await refresh(); }
    catch (e) { error = e; }
    finally { pending = false; }
  }
  async function preparePost(rows: DueItem[]) {
    if (!rows.length) return;
    await action(async () => {
      const impacts: ReconciliationImpactResponse[] = [];
      for (const row of rows) impacts.push(await getPostReconciliationImpact(row.transaction_id!));
      open({ kind: 'post', items: rows, impacts });
    });
  }
  async function confirm() {
    const current = review;
    if (!current || pending || !csrf) return;
    pending = true; dialogError = undefined; notice = '';
    try {
      if (current.kind === 'post') {
        let count = 0;
        for (let i = 0; i < current.items.length; i++) {
          const item = current.items[i];
          const fresh = await getPostReconciliationImpact(item.transaction_id!);
          const approved = current.impacts[i].affected_checkpoints.map(c => c.checkpoint_id).sort();
          const actual = fresh.affected_checkpoints.map(c => c.checkpoint_id).sort();
          if (JSON.stringify(approved) !== JSON.stringify(actual)) { impactChanged = true; return; }
          await postTransaction(item.transaction_id!, csrf, { reconciliation_override: actual.length > 0 });
          count++;
          // A later failure must leave only unposted items in the confirmation.
          review = { kind: 'post', items: current.items.slice(i + 1), impacts: current.impacts.slice(i + 1) };
          selected = selected.filter(id => id !== item.id);
          notice = m.recurring_posted_count({ count: new Intl.NumberFormat(locale).format(count) });
        }
      } else if (current.kind === 'skip') {
        await skipRecurringOccurrence(current.templateID, current.date, reason, csrf); notice = m.recurring_skipped();
      } else if (current.kind === 'discard') {
        await deleteDraftTransaction(current.transactionID, csrf); notice = m.recurring_discarded();
      } else if (current.kind === 'archive') {
        await archiveRecurringTemplate(current.template.id, csrf); notice = m.recurring_archived();
      }
      review = null;
    } catch (e) { dialogError = e; }
    finally { await refresh(); pending = false; }
  }
  async function templateSaved() { review = null; notice = m.recurring_saved(); await refresh(); pending=false; }
  async function draftSaved() { review = null; notice = m.recurring_draft_saved(); await refresh(); pending=false; }
  function amount(value: string, scale: number) { return formatQuantity(value, scale, locale); }
  function commodity(id: number) { return currencies.data?.currencies.find(c => c.id === id)?.code ?? String(id); }
  function occurrenceStatus(status: string) {
    switch (status) { case 'generated': return m.recurring_generated(); case 'blocked': return m.recurring_blocked(); case 'skipped': return m.recurring_skipped_label(); default: return m.recurring_scheduled(); }
  }
  const button = 'min-h-10 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground hover:bg-control-hover focus-visible:outline-2 focus-visible:outline-accent disabled:opacity-50';
</script>

<div class="space-y-5 p-4 sm:p-6">
  <div class="flex flex-wrap items-center justify-between gap-3">
    <div class="flex gap-2" role="group" aria-label={m.recurring_views()}>
      <button class={button} aria-pressed={tab === 'due'} onclick={() => tab = 'due'}>{m.recurring_due()}</button>
      <button class={button} aria-pressed={tab === 'templates'} onclick={() => tab = 'templates'}>{m.recurring_templates()}</button>
    </div>
    <div class="flex gap-2">
      <button class={button} disabled={pending} onclick={() => action(async () => { await refresh(); })}>{m.recurring_refresh()}</button>
      <button class={button} disabled={pending || !csrf || !summary.data} onclick={() => open({kind:'template'})}>{m.recurring_new_template()}</button>
    </div>
  </div>
  {#if notice}<p role="status" class="rounded-(--radius-control) border border-positive p-3 text-sm">{notice}</p>{/if}
  {#if error}<APIFormError {error} />{/if}
  {#if summary.isError}<APIFormError error={summary.error} />{/if}
  {#if tab === 'due'}
    <h2 class="text-lg font-semibold">{m.recurring_due()}</h2>
    <p class="text-sm text-muted">{m.recurring_due_help()}</p>
    {#if due.isPending}<StatePanel title={m.recurring_loading()} copy={m.recurring_due_help()} />
    {:else if due.isError}<StatePanel title={m.recurring_unavailable()} copy={m.recurring_retry_help()}><APIFormError error={due.error} /><button class={button} onclick={() => due.refetch()}>{m.recurring_refresh()}</button></StatePanel>
    {:else if !items.length}<StatePanel title={m.recurring_due_empty()} copy={m.recurring_due_empty_copy()} />
    {:else}
      <button class={button} disabled={pending || !items.some(i => selected.includes(i.id) && i.transaction_id)} onclick={() => preparePost(items.filter(i => selected.includes(i.id) && !!i.transaction_id))}>{m.recurring_post_selected()}</button>
      <div class="space-y-3">
        {#each items as item (item.id)}
          <article class="rounded-(--radius-panel) border border-border bg-surface p-4" aria-labelledby={`occurrence-${item.id}`}>
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div class="min-w-0 space-y-1">
                <h3 id={`occurrence-${item.id}`} class="break-words font-semibold">{item.template_name}</h3>
                <p class="text-sm text-muted"><time datetime={item.occurrence_date}>{displayDate(item.occurrence_date, locale)}</time> · {occurrenceStatus(item.status)}</p>
                <p class="break-words text-sm">{item.payee_name || item.description}</p>
                {#if item.template_archived}<p class="text-sm text-muted">{m.recurring_archived_label()}</p>{/if}
              </div>
              {#if item.transaction_id}<label class="flex min-h-10 items-center gap-2 text-sm"><input type="checkbox" bind:group={selected} value={item.id} disabled={pending} />{m.recurring_select()}</label>{/if}
            </div>
            <ul class="my-3 space-y-1 text-sm tabular-nums">{#each item.amounts as a (a.commodity_id)}<li>{a.commodity_code}: {m.recurring_debit_credit({ debit: amount(a.debit_value,a.quantity_scale), credit: amount(a.credit_value,a.quantity_scale) })}</li>{/each}</ul>
            {#if item.status === 'blocked'}
              <p class="text-sm text-warning">{m.recurring_blocked_help()}</p>
              <details class="my-2 text-sm"><summary class="cursor-pointer">{m.recurring_error_detail()}</summary><p class="break-words">{item.error_summary}</p></details>
            {/if}
            <div class="flex flex-wrap gap-2">
              {#if item.transaction_id}
                <button class={button} disabled={pending || !csrf} onclick={() => preparePost([item])}>{m.recurring_post()}</button>
                <button class={button} disabled={pending || !csrf} onclick={() => action(async () => open({kind:'edit',transaction:await getTransaction(item.transaction_id!)}))}>{m.recurring_edit_draft()}</button>
                <button class={button} disabled={pending || !csrf} onclick={() => open({kind:'discard',transactionID:item.transaction_id!})}>{m.recurring_discard()}</button>
              {:else}
                <button class={button} disabled={pending || !csrf || !item.template_enabled || item.template_archived} onclick={() => action(async () => {const result=await retryRecurringOccurrence(item.template_id,item.occurrence_date,csrf);notice=result.blocked?m.recurring_still_blocked():m.recurring_generated_notice();})}>{m.recurring_retry()}</button>
                <button class={button} disabled={pending || !csrf} onclick={() => open({kind:'skip',templateID:item.template_id,date:item.occurrence_date})}>{m.recurring_skip()}</button>
              {/if}
            </div>
          </article>
        {/each}
      </div>
      {#if due.hasNextPage}<button class={button} disabled={due.isFetchingNextPage} onclick={() => due.fetchNextPage()}>{m.recurring_load_more()}</button>{/if}
    {/if}
  {:else}
    <div class="flex flex-wrap items-center justify-between gap-3"><h2 class="text-lg font-semibold">{m.recurring_templates()}</h2><label class="flex min-h-10 items-center gap-2 text-sm"><input type="checkbox" bind:checked={includeArchived} />{m.recurring_show_archived()}</label></div>
    {#if templates.isPending}<StatePanel title={m.recurring_loading()} copy={m.recurring_template_help()} />
    {:else if templates.isError}<StatePanel title={m.recurring_unavailable()} copy={m.recurring_retry_help()}><APIFormError error={templates.error} /><button class={button} onclick={() => templates.refetch()}>{m.recurring_refresh()}</button></StatePanel>
    {:else if !templates.data?.templates.length}<StatePanel title={m.recurring_templates_empty()} copy={m.recurring_template_help()} />
    {:else}
      <div class="space-y-3">{#each templates.data.templates as template (template.id)}
        <article class="rounded-(--radius-panel) border border-border bg-surface p-4" aria-labelledby={`template-${template.id}`}>
          <h3 id={`template-${template.id}`} class="break-words font-semibold">{template.name}</h3>
          <p class="mt-1 text-sm text-muted">{scheduleLabel(template,locale)} · {template.archived_at ? m.recurring_archived_label() : template.enabled ? m.recurring_enabled() : m.recurring_paused()}</p>
          <p class="mt-1 text-sm">{m.recurring_next_due({ date: template.next_due_on ? displayDate(template.next_due_on,locale) : m.recurring_no_dates() })}</p>
          <ul class="my-2 text-sm tabular-nums">{#each template.postings.filter(p => !p.quantity_value.startsWith('-')) as posting,i (i)}<li>{commodity(posting.commodity_id)} {amount(posting.quantity_value,posting.quantity_scale)}</li>{/each}</ul>
          <div class="flex flex-wrap gap-2">
            {#if !template.archived_at}
              <button class={button} disabled={pending || !csrf} onclick={() => open({kind:'template',template})}>{m.recurring_edit_template()}</button>
              <button class={button} disabled={pending || !csrf} onclick={() => action(async () => {await updateRecurringTemplate(template.id,{enabled:!template.enabled},csrf);notice=m.recurring_saved();})}>{template.enabled?m.recurring_pause():m.recurring_resume()}</button>
              <button class={button} disabled={pending || !csrf || !template.enabled} onclick={() => action(async () => {const result=await runRecurringNow(template.id,csrf);notice=m.recurring_run_result({generated:new Intl.NumberFormat(locale).format(result.generated),blocked:new Intl.NumberFormat(locale).format(result.blocked)});tab='due';})}>{m.recurring_run_now()}</button>
              <button class={button} disabled={pending || !csrf} onclick={() => open({kind:'archive',template})}>{m.recurring_archive()}</button>
            {/if}
            <button class={button} onclick={() => inspecting=inspecting?.id===template.id?undefined:template}>{m.recurring_occurrences()}</button>
          </div>
          {#if inspecting?.id === template.id}
            <section class="mt-4 border-t border-border pt-3" aria-label={m.recurring_occurrences()}>
              <p class="text-sm text-muted">{m.recurring_occurrences_help()}</p>
              {#if occurrences.isPending}<p>{m.recurring_loading()}</p>
              {:else if occurrences.isError}<APIFormError error={occurrences.error} /><button class={button} onclick={() => occurrences.refetch()}>{m.recurring_refresh()}</button>
              {:else if !occurrences.data?.occurrences.length}<p>{m.recurring_no_dates()}</p>
              {:else}<ul class="mt-2 max-h-80 space-y-2 overflow-y-auto">{#each occurrences.data.occurrences as occurrence (occurrence.occurrence_date)}<li class="flex flex-wrap items-center justify-between gap-2 text-sm"><span>{displayDate(occurrence.occurrence_date,locale)} · {occurrenceStatus(occurrence.status)} {occurrence.skip_reason}</span>{#if occurrence.status==='scheduled' && !template.archived_at}<button class={button} disabled={pending || !csrf} onclick={() => open({kind:'skip',templateID:template.id,date:occurrence.occurrence_date})}>{m.recurring_skip()}</button>{/if}</li>{/each}</ul>{/if}
            </section>
          {/if}
        </article>
      {/each}</div>
    {/if}
  {/if}
</div>

<dialog bind:this={dialog} oncancel={(event) => {if(pending) event.preventDefault();else review=null;}} onclose={() => {review=null;}} class="m-auto max-h-[90dvh] w-[calc(100%_-_2rem)] max-w-3xl overflow-y-auto rounded-(--radius-panel) border border-border bg-surface p-5 text-foreground shadow-(--shadow-panel) backdrop:bg-background/80" aria-label={m.recurring_review()}>
  {#if review?.kind === 'template' && summary.data}
    {#key review}<TemplateEditor template={review.template} today={summary.data.today} csrfToken={csrf} onSaved={templateSaved} onCancel={close} onPendingChange={(value) => pending=value} />{/key}
  {:else if review?.kind === 'edit'}
    {#key review.transaction.id}<TransactionEditor mode="edit" transaction={review.transaction} csrfToken={csrf} preservePostings onSaved={draftSaved} onCancel={close} onPendingChange={(value) => pending=value} />{/key}
  {:else if review}
    <h2 class="text-lg font-semibold">{review.kind==='post'?m.recurring_confirm_post():review.kind==='discard'?m.recurring_discard():review.kind==='archive'?m.recurring_archive():m.recurring_skip()}</h2>
    {#if review.kind === 'post'}
      <p class="mt-2 text-sm">{m.recurring_post_help()}</p>
      <ul class="my-3 space-y-2 text-sm">{#each review.items as item (item.id)}<li>{item.template_name} · {displayDate(item.occurrence_date,locale)}{#each item.amounts as a}<p>{a.commodity_code}: {m.recurring_debit_credit({debit:amount(a.debit_value,a.quantity_scale),credit:amount(a.credit_value,a.quantity_scale)})}</p>{/each}</li>{/each}</ul>
      {#each review.impacts as impact}{#if impact.affected_checkpoints.length}<section class="my-3 rounded-(--radius-control) border border-warning p-3"><h3 class="font-semibold">{m.transactions_reconciliation_warning_title()}</h3><p class="text-sm">{m.transactions_reconciliation_warning_copy()}</p><ul class="mt-2 space-y-1 text-sm">{#each impact.affected_checkpoints as cp}<li>{m.transactions_reconciliation_checkpoint_label({account:cp.account_label,commodity:cp.commodity_code,date:displayDate(cp.statement_date,locale)})}</li>{/each}</ul></section>{/if}{/each}
    {:else if review.kind === 'skip'}<p class="my-2 text-sm">{displayDate(review.date,locale)}</p><label class="block text-sm">{m.recurring_reason()}<input class="mt-1 min-h-10 w-full rounded-(--radius-control) border border-border bg-control px-3" bind:value={reason} required maxlength="500" /></label>
    {:else}<p class="my-3 text-sm">{review.kind==='discard'?m.recurring_discard_help():m.recurring_archive_help()}</p>{/if}
    {#if impactChanged}<p role="alert" class="my-3 text-sm text-danger">{m.recurring_impact_changed()}</p>{/if}
    {#if dialogError}<APIFormError error={dialogError} />{/if}
    {#if notice}<p role="status" class="my-2 text-sm">{notice}</p>{/if}
    <div class="mt-4 flex flex-wrap justify-end gap-2"><button class={button} disabled={pending} onclick={close}>{m.recurring_cancel()}</button><button class={button} disabled={pending || impactChanged || !csrf || (review.kind==='skip' && !reason.trim())} onclick={confirm}>{pending?m.recurring_saving():m.recurring_confirm()}</button></div>
  {/if}
</dialog>
