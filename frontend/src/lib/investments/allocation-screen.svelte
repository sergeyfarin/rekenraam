<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { beforeNavigate, goto } from '$app/navigation';
  import { page } from '$app/state';
  import { parseISO } from 'date-fns';
  import PageHeader from '#lib/components/page-header.svelte';
  import Panel from '#lib/components/panel.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import ModalDialog from '#lib/components/modal-dialog.svelte';
  import { allocationQueryKey, allocationQueryOptions, saveAllocation, type AllocationResponse, type AllocationRevision } from '#lib/api/allocation.ts';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import { formatEditableAmount } from '#lib/money/amount.ts';
  import { formatQuantity } from '#lib/money/format.ts';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { m } from '#lib/paraglide/messages.js';
  import { allocationIDs, parseTargetUnits } from './allocation-model.ts';
  import { formatWeight, returnReasonText } from './returns-model.ts';
  import AllocationChart from './allocation-chart.svelte';

  const locale = $derived(getLocale());
  const client = useQueryClient();
  const session = createQuery(() => authSessionQueryOptions());
  const query = $derived({
    from: page.url.searchParams.get('from') || undefined,
    to: page.url.searchParams.get('to') || undefined,
    reporting_currency_id: page.url.searchParams.has('reporting_currency_id') ? Number(page.url.searchParams.get('reporting_currency_id')) : undefined
  });
  const allocation = createQuery(() => ({ ...allocationQueryOptions(query), notifyOnChangeProps: 'all' as const }));
  const data = $derived(allocation.data);
  let selectedDate = $state('');
  let selectedInstrument = $state<number | null>(null);
  let selectedRevision = $state<number | null>(null);
  let showMarkers = $state(false);
  const point = $derived(data?.points.find(p => p.date === selectedDate) ?? data?.points.at(-1));
  const ids = $derived(data ? allocationIDs(data.points, data.current.targets) : []);
  const picked = $derived(point?.rows.filter(r => r.commodity_id === (selectedInstrument ?? ids[0])) ?? []);
  const fieldClass = 'min-h-11 min-w-0 rounded-(--radius-control) border border-border bg-control px-3 text-foreground';
  const buttonClass = `${fieldClass} text-sm font-semibold hover:bg-control-hover disabled:opacity-50`;
  const primaryButtonClass = 'min-h-11 rounded-(--radius-control) border border-border bg-accent px-3 text-sm font-semibold text-accent-foreground disabled:opacity-50';

  // Fields, choices and concurrency token are captured together only on an
  // explicit open/reload. Query refetches cannot accept a newer token for old fields.
  let editing = $state(false);
  let discardAction = $state<(() => void) | null>(null);
  let editSnapshot = $state.raw<AllocationResponse | null>(null);
  const editAccountIDs = $derived(editSnapshot ? [...new Set([...editSnapshot.accounts.map(a => a.id), ...editSnapshot.current.account_ids])] : []);
  let accountDraft = $state<number[]>([]);
  let targetDraft = $state<{ commodityID: number; text: string }[]>([]);
  let addID = $state('');
  let dirty = $state(false);
  let editLocale = $state('en');
  let pending = $state(false);
  let saveError = $state<unknown>();
  let saved = $state(false);
  const parsedTargets = $derived(targetDraft.map(t => ({ commodity_id: t.commodityID, units: parseTargetUnits(t.text, editLocale) })));
  const totalUnits = $derived(parsedTargets.reduce((sum, t) => sum + (t.units ?? 0), 0));
  const invalid = $derived(parsedTargets.some(t => t.units === null) || (targetDraft.length > 0 && totalUnits !== 1000000) || (editSnapshot?.current.targets.length !== 0 && targetDraft.length === 0));
  const choices = $derived(editSnapshot ? [{ id: 0, name: m.allocation_cash() }, ...editSnapshot.commodities.filter(c => c.targetable).map(c => ({ id: c.id, name: c.name || c.code }))].filter(c => !targetDraft.some(t => t.commodityID === c.id)) : []);

  function accountName(id: number, source = data): string {
    const account = source?.accounts.find(a => a.id === id);
    return account?.name || account?.code || `${m.accounts_unnamed_account({ id })}${account ? '' : ` · ${m.allocation_unavailable()}`}`;
  }
  function instrumentName(id: number): string {
    if (id === 0) return m.allocation_cash();
    const commodity = data?.commodities.find(c => c.id === id) ?? editSnapshot?.commodities.find(c => c.id === id);
    return commodity?.name || commodity?.code || `#${id}`;
  }
  const percent = (units: number) => new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 4 }).format(units / 1000000);
  const drift = (ratio: string) => m.allocation_drift_pp({ value: new Intl.NumberFormat(locale, { minimumFractionDigits: 2, maximumFractionDigits: 4, signDisplay: 'exceptZero' }).format(Number(ratio) * 100) });
  const day = (date: string) => new Intl.DateTimeFormat(locale, { dateStyle: 'medium' }).format(parseISO(date));
  const instant = (timestamp: string) => new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', timeZoneName: 'short', timeZone: data?.time_zone }).format(new Date(timestamp));
  function money(amount: { value: string; scale: number; commodity_id: number } | null): string {
    if (!amount) return m.allocation_unavailable();
    return `${formatQuantity(amount.value, amount.scale, locale)} ${data?.commodities.find(c => c.id === amount.commodity_id)?.code ?? `#${amount.commodity_id}`}`;
  }
  function openEditor() {
    if (!data || pending) return;
    const source = data;
    guardDiscard(() => loadEditor(source));
  }
  function loadEditor(source: AllocationResponse) {
    editLocale = locale;
    editSnapshot = structuredClone(source);
    accountDraft = [...source.current.account_ids];
    targetDraft = source.current.targets.map(t => ({ commodityID: t.commodity_id, text: formatEditableAmount(String(t.units), 4, locale) }));
    addID = ''; dirty = false; saveError = undefined; editing = true;
  }
  function guardDiscard(action: () => void) {
    if (pending) return;
    if (dirty) discardAction = action;
    else action();
  }
  function closeEditor() {
    guardDiscard(() => { editing = false; dirty = false; });
  }
  function discard() {
    const action = discardAction;
    discardAction = null; dirty = false;
    action?.();
  }
  function toggleAccount(id: number, checked: boolean) {
    accountDraft = checked ? [...accountDraft, id] : accountDraft.filter(a => a !== id); dirty = true;
  }
  function addTarget() {
    if (addID === '') return;
    targetDraft = [...targetDraft, { commodityID: Number(addID), text: '0' }]; addID = ''; dirty = true;
  }
  async function save() {
    if (!editSnapshot || invalid || pending) return;
    pending = true; saveError = undefined;
    try {
      await saveAllocation({ expected_revision: editSnapshot.current.id, account_ids: accountDraft, targets: parsedTargets.map(t => ({ commodity_id: t.commodity_id, units: t.units! })) }, session.data?.csrf_token ?? '');
      editing = false; dirty = false; saved = true;
      await client.invalidateQueries({ queryKey: allocationQueryKey });
    } catch (error) { saveError = error; }
    finally { pending = false; }
  }
  async function apply(event: SubmitEvent & { currentTarget: HTMLFormElement }) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const params = new URLSearchParams();
    for (const key of ['from', 'to', 'reporting_currency_id']) { const value = String(form.get(key) ?? ''); if (value) params.set(key, value); }
    await goto(`${page.url.pathname}?${params}`, { reset: false });
  }
  beforeNavigate((navigation) => {
    if (pending) { navigation.cancel(); return; }
    // Leaving the document uses the browser's beforeunload protection.
    if (!dirty || navigation.willUnload) return;
    navigation.cancel();
    const destination = navigation.to?.url;
    if (destination) guardDiscard(() => {
      editing = false;
      if (navigation.type === 'popstate' && navigation.delta) history.go(navigation.delta);
      else void goto(destination.href);
    });
  });
  function beforeUnload(event: BeforeUnloadEvent) { if (dirty || pending) { event.preventDefault(); event.returnValue = ''; } }
  function previousRevision(index: number): AllocationRevision | undefined { return data?.revisions[index + 1]; }
</script>

<svelte:window onbeforeunload={beforeUnload} />
<PageHeader title={m.allocation_title()} copy={m.allocation_intro()}>
  {#snippet actions()}<button type="button" class={buttonClass} disabled={!data || pending || allocation.isError} onclick={openEditor}>{m.allocation_edit()}</button>{/snippet}
</PageHeader>
<div class="space-y-4">
  {#if allocation.isPending}<Panel><p role="status">{m.allocation_loading()}</p></Panel>
  {:else if allocation.isError}<Panel><APIFormError error={allocation.error} /><button class={buttonClass} type="button" onclick={() => allocation.refetch()}>{m.allocation_retry()}</button></Panel>
  {:else if data && point}
    <Panel>
      <form class="grid gap-3 sm:grid-cols-4" onsubmit={apply}>
        <label class="grid min-w-0 gap-1 text-sm">{m.allocation_from()}<input class={fieldClass} type="date" name="from" value={data.from} required /></label>
        <label class="grid min-w-0 gap-1 text-sm">{m.allocation_to()}<input class={fieldClass} type="date" name="to" value={data.to} required /></label>
        <label class="grid min-w-0 gap-1 text-sm">{m.allocation_currency()}<select class={fieldClass} name="reporting_currency_id" value={data.reporting_currency_id}>{#each data.commodities.filter(c => c.kind === 'currency') as c (c.id)}<option value={c.id}>{c.code}</option>{/each}</select></label>
        <button class={`${buttonClass} self-end`} type="submit">{m.allocation_apply()}</button>
      </form>
    </Panel>
    <Panel>
      <h2 class="text-lg font-semibold text-foreground">{m.allocation_pool()}</h2>
      <p class="mt-1 text-2xl font-semibold tabular-nums text-foreground" data-testid="allocation-total">{money(data.points.at(-1)?.total ?? null)}</p>
      <p class="mt-2 text-sm text-muted">{m.allocation_scope_help()}</p>
      {#if data.current.id === 0}<p role="status" class="mt-2 text-sm text-warning">{m.allocation_scope_suggested()}</p>{/if}
      <details class="mt-3"><summary class="min-h-11 cursor-pointer text-sm font-semibold text-foreground">{m.allocation_scope_details({ included: new Intl.NumberFormat(locale).format(data.current.account_ids.length), excluded: new Intl.NumberFormat(locale).format(data.accounts.filter(a => !a.included).length) })}</summary>
        <div class="grid gap-3 text-sm sm:grid-cols-2">
          <div><h3 class="font-semibold text-foreground">{m.allocation_included()}</h3><ul class="text-muted">{#each data.current.account_ids as id (id)}<li class="wrap-anywhere">{accountName(id)}</li>{/each}</ul></div>
          <div><h3 class="font-semibold text-foreground">{m.allocation_excluded()}</h3><ul class="text-muted">{#each data.accounts.filter(a => !a.included) as account (account.id)}<li class="wrap-anywhere">{accountName(account.id)}</li>{/each}</ul></div>
        </div>
      </details>
      <a class="mt-2 inline-flex min-h-11 items-center text-sm font-semibold text-accent underline" href={`/app/reports/net-worth?start_date=${data.from}&end_date=${data.to}&reporting_currency_id=${data.reporting_currency_id}`}>{m.allocation_net_worth()}</a>
      {#if saved}<p role="status" class="text-sm text-positive">{m.allocation_saved()}</p>{/if}
      {#if data.current.targets.length === 0}<p role="status" class="text-sm text-muted">{m.allocation_no_targets()}</p>{/if}
      {#if data.current.account_ids.length === 0}<p class="text-sm text-muted">{m.allocation_empty_pool()}</p>{/if}
    </Panel>
    <Panel>
      <AllocationChart {data} label={instrumentName} {percent} {showMarkers} formatDay={day} formatSaved={instant} onRevision={(id) => { selectedRevision = id; document.getElementById(`allocation-revision-${id}`)?.focus(); }} />
      <label class="mt-3 flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" bind:checked={showMarkers} />{m.allocation_show_changes()}</label>
      <p class="mt-2 text-sm text-muted">{m.allocation_current_help()}</p>
      <p class="mt-2 text-sm text-muted">{m.allocation_valuation_window({ days: new Intl.NumberFormat(locale).format(data.valuation.max_staleness_days) })}</p>
      <div class="mt-4 grid gap-3 sm:grid-cols-2">
        <label class="grid min-w-0 gap-1 text-sm">{m.allocation_selected_date()}<select class={fieldClass} value={point.date} onchange={(e) => { selectedDate = e.currentTarget.value; }}>{#each data.points as p (p.date)}<option value={p.date}>{day(p.date)}</option>{/each}</select></label>
        <label class="grid min-w-0 gap-1 text-sm">{m.allocation_instrument()}<select class={fieldClass} value={selectedInstrument ?? ids[0]} onchange={(e) => { selectedInstrument = Number(e.currentTarget.value); }}>{#each ids as id (id)}<option value={id}>{instrumentName(id)}</option>{/each}</select></label>
      </div>
      <div role="status" class="mt-3 space-y-1 text-sm text-foreground" data-testid="allocation-inspection">
        {#each picked as row (`${row.commodity_id}-${row.side}`)}<p class="wrap-anywhere">{instrumentName(row.commodity_id)} · {m.allocation_actual()}: {row.weight === null ? m.allocation_unavailable() : formatWeight(row.weight, locale)} · {m.allocation_target()}: {row.target_units === null ? m.allocation_no_target() : percent(row.target_units)} · {m.allocation_drift()}: {row.drift === null ? m.allocation_unavailable() : drift(row.drift)}{#if row.side === 'short'} · {m.returns_allocation_short()}{/if}</p>{/each}
      </div>
      {#if point.negative_cash === null || point.negative_cash.value !== '0'}<p class="mt-3 text-sm text-warning" data-testid="allocation-negative-cash">{m.allocation_negative_cash({ value: money(point.negative_cash) })}</p>{/if}
      {#if point.reason}<p role="status" class="mt-3 text-sm text-warning">{returnReasonText(point.reason, point.date, day)}</p>{/if}
      {#if point.chart_reason === 'signed_allocation'}<p role="status" class="mt-3 text-sm text-warning">{m.allocation_signed()}</p>{/if}
      <p id="allocation-grouping-help" class="mt-3 text-sm text-muted">{m.allocation_grouping_help()}</p>
      <!-- svelte-ignore a11y_no_noninteractive_tabindex (this region permits keyboard scrolling of the exact allocation table) -->
      <div class="mt-3 overflow-x-auto" role="region" tabindex="0" aria-label={m.allocation_table()} aria-describedby="allocation-grouping-help">
        <table class="w-full text-left text-sm" data-testid="allocation-table">
          <caption class="py-2 text-left font-semibold text-foreground">{m.allocation_table()} · {day(point.date)}</caption>
          <thead><tr class="text-muted">{#each [m.allocation_instrument(), m.allocation_value(), m.allocation_actual(), m.allocation_target(), m.allocation_drift()] as heading}<th scope="col" class="px-2 py-2 font-medium">{heading}</th>{/each}</tr></thead>
          <tbody>{#each point.rows as row (`${row.commodity_id}-${row.side}`)}<tr class="border-t border-border text-foreground"><th scope="row" class="min-w-28 px-2 py-3 font-medium wrap-anywhere">{instrumentName(row.commodity_id)}{#if row.side === 'short'} · {m.returns_allocation_short()}{/if}</th><td class="px-2 py-3 tabular-nums">{money(row.amount)}</td><td class="px-2 py-3 tabular-nums">{row.weight === null ? m.allocation_unavailable() : formatWeight(row.weight, locale)}</td><td class="px-2 py-3 tabular-nums">{row.target_units === null ? m.allocation_no_target() : percent(row.target_units)}</td><td class="px-2 py-3 tabular-nums">{row.drift === null ? m.allocation_unavailable() : drift(row.drift)}</td></tr>{/each}</tbody>
        </table>
      </div>
      {#if point.unknown.length}<h3 class="mt-3 text-sm font-semibold text-foreground">{m.returns_allocation_unknown_title()}</h3><ul class="text-sm text-muted">{#each point.unknown as item (`${item.account_id}-${item.commodity_id}`)}<li class="wrap-anywhere">{accountName(item.account_id)} · {formatQuantity(item.value, item.scale, locale)} {instrumentName(item.commodity_id)}</li>{/each}</ul>{/if}
      {#if data.valuation.rates_used.some(r => r.observation_date !== r.requested_date)}<p class="mt-3 text-sm text-muted">{m.allocation_stale()}</p>{/if}
    </Panel>
    <Panel>
      <h2 class="text-sm font-semibold text-foreground">{m.returns_allocation_by_currency()} · {day(data.to)}</h2>
      <p class="mt-1 text-sm text-muted">{m.allocation_currency_help()}</p>
      {#if data.currency_allocation.basis === 'known_only'}<p class="mt-2 text-sm text-warning">{m.returns_allocation_known_only()}</p>{/if}
      <ul class="mt-2 space-y-2 text-sm text-foreground">{#each data.currency_allocation.by_currency as row, index (index)}<li>{row.currency_id === null ? m.returns_allocation_no_currency() : data.commodities.find(c => c.id === row.currency_id)?.code ?? `#${row.currency_id}`} · {row.weight === null ? m.allocation_unavailable() : formatWeight(row.weight, locale)}{#if row.side === 'short'} · {m.returns_allocation_short()}{/if}</li>{/each}</ul>
    </Panel>
    <Panel>
      <h2 id="allocation-history" class="text-base font-semibold text-foreground">{m.allocation_history()}</h2>
      <p class="mt-1 text-sm text-muted">{m.allocation_history_help()}</p>
      {#if data.revisions.length === 0}<p class="mt-2 text-sm text-muted">{m.allocation_no_history()}</p>{/if}
      <ol class="mt-3 space-y-3">{#each data.revisions as revision, index (revision.id)}
        {@const previous = previousRevision(index)}
        <li><details open={selectedRevision === revision.id}><summary id={`allocation-revision-${revision.id}`} class="min-h-11 cursor-pointer text-sm font-semibold text-foreground">{m.allocation_saved_change({ date: instant(revision.saved_at) })}</summary>
          <p class="my-2 text-sm text-muted">{m.allocation_pool()}: {revision.account_ids.map(id => accountName(id)).join(', ') || m.allocation_empty_pool()}</p>
          <ul class="space-y-1 text-sm text-foreground">{#each [...new Set([...revision.targets.map(t => t.commodity_id), ...(previous?.targets.map(t => t.commodity_id) ?? [])])] as id (id)}<li class="wrap-anywhere">{instrumentName(id)} · {m.allocation_previous()}: {previous?.targets.length ? percent(previous.targets.find(t => t.commodity_id === id)?.units ?? 0) : m.allocation_no_target()} · {m.allocation_new()}: {revision.targets.length ? percent(revision.targets.find(t => t.commodity_id === id)?.units ?? 0) : m.allocation_no_target()}</li>{/each}</ul>
        </details></li>
      {/each}</ol>
    </Panel>
  {/if}
</div>

{#if editing && editSnapshot}
  <ModalDialog label={m.allocation_edit()} {pending} onCancel={closeEditor} class="w-full max-w-2xl rounded-(--radius-panel) border border-border bg-surface p-5 text-foreground shadow-(--shadow-panel)">
    <form class="space-y-4" onsubmit={(e) => { e.preventDefault(); void save(); }} aria-busy={pending}>
      <h2 class="text-lg font-semibold">{m.allocation_edit()}</h2>
      <p class="text-sm text-muted">{m.allocation_edit_help()}</p>
      <APIFormError error={saveError} />
      {#if saveError}<button class={buttonClass} type="button" disabled={pending || allocation.isFetching} onclick={async () => { const fresh = await allocation.refetch(); if (fresh.data) { const source = fresh.data; guardDiscard(() => loadEditor(source)); } }}>{m.allocation_reload()}</button>{/if}
      <fieldset disabled={pending} class="space-y-2"><legend class="text-sm font-semibold">{m.allocation_included()}</legend>
        <div class="max-h-64 space-y-1 overflow-y-auto rounded-(--radius-control) border border-border p-3">
          {#each editAccountIDs as id (id)}<label class="flex min-h-11 items-start gap-2 py-2 text-sm"><input class="mt-1" type="checkbox" checked={accountDraft.includes(id)} onchange={(e) => toggleAccount(id, e.currentTarget.checked)} /><span class="wrap-anywhere">{accountName(id, editSnapshot)}</span></label>{/each}
        </div>
      </fieldset>
      <fieldset disabled={pending} class="space-y-3"><legend class="text-sm font-semibold">{m.allocation_targets()}</legend>
        {#each targetDraft as target, index (target.commodityID)}<div class="flex items-end gap-2"><label class="grid min-w-0 flex-1 gap-1 text-sm"><span class="wrap-anywhere">{instrumentName(target.commodityID)} · {m.allocation_target()}</span><input class={fieldClass} inputmode="decimal" value={target.text} required oninput={(e) => { targetDraft[index].text = e.currentTarget.value; dirty = true; }} /></label><button class={buttonClass} type="button" onclick={() => { targetDraft = targetDraft.filter(t => t.commodityID !== target.commodityID); dirty = true; }} aria-label={m.allocation_remove_named({ name: instrumentName(target.commodityID) })}>{m.allocation_remove()}</button></div>{/each}
        <div class="flex items-end gap-2"><label class="grid min-w-0 flex-1 gap-1 text-sm">{m.allocation_add_instrument()}<select class={fieldClass} bind:value={addID}><option value="">{m.allocation_choose()}</option>{#each choices as choice (choice.id)}<option value={String(choice.id)}>{choice.name}</option>{/each}</select></label><button class={buttonClass} type="button" disabled={addID === ''} onclick={addTarget}>{m.allocation_add()}</button></div>
        <p class="text-sm tabular-nums" role="status">{m.allocation_total_target({ value: percent(totalUnits) })}</p>
        {#if invalid}<p class="text-sm text-warning">{m.allocation_target_validation()}</p>{/if}
      </fieldset>
      <div class="flex flex-wrap justify-end gap-2"><button class={buttonClass} type="button" disabled={pending} onclick={closeEditor}>{m.allocation_cancel()}</button><button class={primaryButtonClass} type="submit" disabled={pending || invalid || !session.data?.csrf_token}>{pending ? m.allocation_saving() : m.allocation_save()}</button></div>
    </form>
  </ModalDialog>
{/if}

{#if discardAction}
  <ModalDialog role="alertdialog" label={m.allocation_discard()} onCancel={() => { discardAction = null; }} class="max-w-md p-5">
    <h2 class="text-lg font-semibold text-foreground">{m.allocation_discard()}</h2>
    <div class="mt-4 flex flex-wrap justify-end gap-2">
      <button class={buttonClass} type="button" data-initial-focus onclick={() => { discardAction = null; }}>{m.allocation_keep_editing()}</button>
      <button class={primaryButtonClass} type="button" onclick={discard}>{m.allocation_discard_confirm()}</button>
    </div>
  </ModalDialog>
{/if}
