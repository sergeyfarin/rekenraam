<script lang="ts">
  import ObservationStatus from '#lib/valuation/observation-status.svelte';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { untrack, tick } from 'svelte';
  import { page } from '$app/state';
  import { createQuery, createInfiniteQuery } from '@tanstack/svelte-query';
  import { pricingWorkspaceQueryOptions, priceHistoryQueryOptions, type PriceObservation, type PricingWorkspace } from '#lib/api/price-management.ts';
  import Panel from '#lib/components/panel.svelte';
  import StatePanel from '#lib/components/state-panel.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import StatusBadge from '#lib/components/status-badge.svelte';
  import PriceDetail from './price-detail.svelte';
  import PriceForm from './price-form.svelte';
  import { coverageFilter, filterCoverage } from './coverage-filter.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  const ownerToday = useOwnerToday();

  function pairID(value: string | null) { const id = Number(value); return Number.isSafeInteger(id) && id > 0 ? id : 0; }
  let asOf = $state('');
  let base = $state(untrack(() => pairID(page.url.searchParams.get('base_commodity_id'))));
  let quote = $state(untrack(() => pairID(page.url.searchParams.get('quote_commodity_id'))));
  let includeVoided = $state(true);
  let modal = $state<{ mode: 'create' | 'correct' | 'void'; target: PriceObservation | null; workspace: PricingWorkspace } | null>(null);
  let trigger = $state<HTMLElement | null>(null);
  let success = $state(false);
  const workspaceQuery = createQuery(() => pricingWorkspaceQueryOptions(asOf || ownerToday()));
  const historyQuery = createInfiniteQuery(() => priceHistoryQueryOptions(base, quote, includeVoided));
  const workspace = $derived(workspaceQuery.data);
  // Home's status line links here filtered to what it counted (#476); read on
  // every navigation so a second link from the same page applies.
  const statusFilter = $derived(coverageFilter(page.url.searchParams.get('status')));
  const coverage = $derived(filterCoverage(workspace?.coverage ?? [], statusFilter));
  const unfilteredHref = $derived.by(() => { const params = new URLSearchParams(page.url.searchParams.toString()); params.delete('status'); const query = params.toString(); return `${page.url.pathname}${query ? `?${query}` : ''}`; });
  const codes = $derived(new Map((workspace?.commodities ?? []).map(item => [item.id, item.code])));
  const history = $derived(historyQuery.data?.pages.flatMap(item => item.prices) ?? []);
  const locale = $derived(getLocale());
  function open(mode: 'create' | 'correct' | 'void', target: PriceObservation | null = null) {
    if (!workspace) return;
    trigger = document.activeElement as HTMLElement; success = false;
    // The dialog keeps its opening snapshot while server today/coverage refresh.
    modal = { mode, target, workspace };
  }
  function close() { modal = null; void tick().then(() => trigger?.focus()); }
  function saved(savedBase: number, savedQuote: number) { base = savedBase; quote = savedQuote; success = true; close(); }
  async function refresh() { await Promise.all([workspaceQuery.refetch(), ...(base > 0 && quote > 0 && base !== quote ? [historyQuery.refetch()] : [])]); }
</script>

<div class="space-y-5">
  <p class="text-sm text-muted"><a href="https://www.coingecko.com/en/api" target="_blank" rel="noreferrer" class="underline">{m.quotes_attribution()}</a></p>
  <div class="flex flex-wrap items-center gap-3">
    <button type="button" onclick={() => open('create')} disabled={!workspace || workspaceQuery.isError || workspaceQuery.isFetching} class="min-h-11 rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background disabled:opacity-50">{m.pricing_add()}</button>
    <button type="button" onclick={refresh} disabled={workspaceQuery.isFetching} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold disabled:opacity-50">{m.pricing_refresh()}</button>
    <a href="/app/settings/currencies" class="text-sm font-semibold underline underline-offset-4">{m.pricing_provider_settings()}</a>
  </div>
  {#if success}<p role="status" class="text-sm font-semibold">{m.pricing_saved()}</p>{/if}
  {#if workspaceQuery.isPending}
    <StatePanel title={m.pricing_loading()} copy={m.pricing_copy()} />
  {:else if workspaceQuery.isError}
    <StatePanel title={m.pricing_error()} copy={m.pricing_copy()}><APIFormError error={workspaceQuery.error} /></StatePanel>
  {:else if workspace}
    <Panel>
      <h2 class="text-lg font-semibold">{m.pricing_coverage()}</h2>
      <p class="mt-2 text-sm text-muted">{m.pricing_coverage_copy({ days: new Intl.NumberFormat(locale).format(Math.min(workspace.staleness_max_days, workspace.valuation_window_days)) })}</p>
      <p class="mt-2 text-sm text-muted">{m.pricing_consumer_limits({ policy: new Intl.NumberFormat(locale).format(workspace.staleness_max_days), window: new Intl.NumberFormat(locale).format(workspace.valuation_window_days) })}</p>
      <p class="mt-2 text-sm text-muted">{m.valuation_report_policy()}</p>
      <label class="mt-4 grid max-w-xs gap-1 text-sm" for="pricing-as-of">{m.pricing_as_of()}<input id="pricing-as-of" type="date" value={asOf || workspace.as_of} onchange={(event) => { asOf = event.currentTarget.value; }} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3" /></label>
      {#if workspaceQuery.isFetching}<p role="status" class="mt-3 text-sm">{m.pricing_loading()}</p>{/if}
      {#if statusFilter}<p class="mt-4 text-sm"><span class="font-semibold">{statusFilter === 'unusable' ? m.pricing_filter_unusable() : m.pricing_filter_stale()}</span> <a href={unfilteredHref} class="underline underline-offset-4">{m.pricing_filter_clear()}</a></p>{/if}
      {#if workspace.coverage.length === 0}<p class="mt-4 text-sm text-muted">{m.pricing_coverage_empty()}</p>
      {:else if coverage.length === 0}<p class="mt-4 text-sm text-muted">{m.pricing_filter_empty()}</p>
      {:else}<ul class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-3">{#each coverage as item}
        <li class="min-w-0 space-y-2 rounded-(--radius-panel) border border-border p-3">
          <button type="button" onclick={() => { base = item.base_commodity_id; quote = item.quote_commodity_id; }} class="min-h-11 max-w-full wrap-anywhere text-left font-semibold underline underline-offset-4">{m.pricing_pair({ base: codes.get(item.base_commodity_id) ?? '', quote: codes.get(item.quote_commodity_id) ?? '' })}</button>
          <div><StatusBadge tone={item.status === 'usable' ? 'positive' : 'warning'}>{item.status === 'usable' ? m.pricing_usable() : item.status === 'stale' ? m.pricing_stale() : m.pricing_missing()}</StatusBadge></div>
          {#if item.age_days !== undefined}<p class="text-sm text-muted">{m.pricing_age({ days: new Intl.NumberFormat(locale).format(item.age_days) })}</p>{/if}
          {#if item.observation}<PriceDetail price={item.observation} {workspace} />{#if item.status === 'missing'}<ObservationStatus status={item.reason} />{/if}{:else}<p class="text-sm text-muted">{m.pricing_missing_copy()}</p><ObservationStatus status={item.reason} />{/if}
        </li>
      {/each}</ul>{/if}
    </Panel>
    <Panel>
      <h2 class="text-lg font-semibold">{m.pricing_history()}</h2>
      <p class="mt-2 text-sm text-muted">{m.pricing_history_copy()}</p>
      <div class="mt-4 grid gap-3 sm:grid-cols-2">
        <label class="grid min-w-0 gap-1 text-sm" for="pricing-history-base">{m.pricing_base()}<select id="pricing-history-base" bind:value={base} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3"><option value={0}>{m.pricing_choose()}</option>{#each workspace.commodities as commodity}<option value={commodity.id}>{commodity.code}</option>{/each}</select></label>
        <label class="grid min-w-0 gap-1 text-sm" for="pricing-history-quote">{m.pricing_quote()}<select id="pricing-history-quote" bind:value={quote} class="min-h-11 min-w-0 w-full rounded-(--radius-control) border border-border bg-control px-3"><option value={0}>{m.pricing_choose()}</option>{#each workspace.commodities.filter(item => item.kind === 'currency') as commodity}<option value={commodity.id}>{commodity.code}</option>{/each}</select></label>
      </div>
      <label class="mt-3 flex min-h-11 items-center gap-2 text-sm"><input type="checkbox" bind:checked={includeVoided} />{m.pricing_include_retired()}</label>
      {#if base <= 0 || quote <= 0 || base === quote}<p class="mt-4 text-sm text-muted">{m.pricing_choose_pair()}</p>
      {:else if historyQuery.isPending}<p role="status" class="mt-4 text-sm">{m.pricing_loading()}</p>
      {:else}
        {#if historyQuery.isError}<div class="mt-4"><APIFormError error={historyQuery.error} /><button type="button" onclick={() => historyQuery.refetch()} class="min-h-11 font-semibold underline">{m.pricing_refresh()}</button></div>{/if}
        {#if history.length === 0 && !historyQuery.isError}<p class="mt-4 text-sm text-muted">{m.pricing_history_empty()}</p>{/if}
        <ul class="mt-4 space-y-4">{#each history as price (price.id)}
          <li data-price-id={price.id} class="min-w-0 space-y-3 rounded-(--radius-panel) border border-border p-3">
            <PriceDetail {price} {workspace} detailed />
            {#if !price.voided_at}<div class="flex flex-wrap gap-3">
              <button type="button" onclick={() => open('correct', price)} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 text-sm font-semibold">{m.pricing_correct()}</button>
              <button type="button" onclick={() => open('void', price)} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 text-sm font-semibold">{m.pricing_void()}</button>
            </div>{/if}
          </li>
        {/each}</ul>
        {#if historyQuery.hasNextPage}<button type="button" onclick={() => historyQuery.fetchNextPage()} disabled={historyQuery.isFetchingNextPage} class="mt-4 min-h-11 rounded-(--radius-control) border border-border bg-control px-4 text-sm font-semibold disabled:opacity-50">{historyQuery.isFetchingNextPage ? m.pricing_loading() : m.pricing_load_more()}</button>{/if}
      {/if}
    </Panel>
  {/if}
  {#if modal}<PriceForm workspace={modal.workspace} mode={modal.mode} target={modal.target} initialBase={base} initialQuote={quote} onSaved={saved} onCancel={close} />{/if}
</div>
