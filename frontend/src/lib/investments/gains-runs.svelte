<script lang="ts">
  import { createInfiniteQuery, createQuery } from '@tanstack/svelte-query';
  import { getGainsRuns, getGainsRun, gainsRunsQueryKey, gainsRunQueryKey } from '#lib/api/investments.ts';
  import Panel from '#lib/components/panel.svelte';
  import GainsStatement from './gains-statement.svelte';
  import PositionSideBadge from './position-side-badge.svelte';
  import ObservationStatus from '#lib/valuation/observation-status.svelte';
  import { formatMoney, formatQuantity, joinCommodityAmount } from '#lib/money/format.ts';
  import { negateCoefficient } from '#lib/money/amount.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  let { selectedID = $bindable(0) }: { selectedID?: number } = $props();
  const locale = $derived(getLocale());
  const dates = $derived(new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }));
  const list = createInfiniteQuery(() => ({
    queryKey: gainsRunsQueryKey, initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => getGainsRuns(pageParam), getNextPageParam: (last) => last.next_cursor ?? undefined
  }));
  const runs = $derived(list.data?.pages.flatMap((page) => page.runs) ?? []);
  const query = createQuery(() => ({ queryKey: gainsRunQueryKey(selectedID), queryFn: () => getGainsRun(selectedID), enabled: selectedID > 0, staleTime: Infinity }));
  const run = $derived(query.data);

  const days = $derived(new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }));
  function day(date: string): string { return days.format(new Date(`${date}T00:00:00Z`)); }

  function amount(value: string | null | undefined, scale: number | null | undefined, currencyID: number): string {
    if (value == null || scale == null) return m.valuation_statement_absent();
    const currency = run?.output.currencies.find((currency) => currency.id === currencyID);
    return joinCommodityAmount(currency?.code ?? `#${currencyID}`, formatMoney(value, scale, currency?.standard_scale ?? scale, locale));
  }


</script>

<section id="saved-gains" class="min-w-0 space-y-3" aria-label={m.valuation_runs_title()} data-testid="gains-runs">
  <h2 class="text-sm font-semibold text-foreground">{m.valuation_runs_title()}</h2>
  <p class="text-sm text-muted">{m.valuation_runs_note()}</p>
  {#if list.isPending}<p role="status" class="text-sm text-muted">{m.valuation_runs_loading()}</p>
  {:else if list.isError}<p role="alert" class="text-sm text-danger">{m.valuation_runs_load_error()}</p>
    <button type="button" class="min-h-11 text-sm underline" onclick={() => list.refetch()}>{m.valuation_runs_retry()}</button>
  {:else if runs.length === 0}<p class="text-sm text-muted">{m.valuation_runs_empty()}</p>
  {:else}
    <ul class="grid gap-2 sm:grid-cols-2">
      {#each runs as item (item.id)}
        <li><button type="button" aria-pressed={selectedID === item.id} onclick={() => { selectedID = item.id; }}
          class="min-h-11 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-left text-sm text-foreground hover:bg-control-hover">
          {m.valuation_runs_item({ id: String(item.id), kind: item.report_type === 'statement' ? m.valuation_statement_title() : m.investments_tab_gains(), date: dates.format(new Date(item.captured_at)) })}
          <span class="block text-xs text-muted">{item.inputs.from ? day(item.inputs.from) : '…'} – {item.inputs.to ? day(item.inputs.to) : '…'} · {day(item.inputs.profile.valuation_as_of)}</span>
        </button></li>
      {/each}
    </ul>
    {#if list.hasNextPage}<button type="button" disabled={list.isFetchingNextPage} onclick={() => list.fetchNextPage()} class="min-h-11 text-sm text-foreground underline">{m.valuation_runs_more()}</button>{/if}
  {/if}
  {#if selectedID > 0}
    <div class="min-w-0 space-y-3 border-t border-border pt-3" data-testid="saved-gains-run">
      {#if query.isPending}<p role="status" class="text-sm text-muted">{m.valuation_runs_loading()}</p>
      {:else if query.isError}<p role="alert" class="text-sm text-danger">{m.valuation_runs_load_error()}</p>
        <button type="button" class="min-h-11 text-sm underline" onclick={() => query.refetch()}>{m.valuation_runs_retry()}</button>
      {:else if run}
        <h3 class="text-sm font-semibold text-foreground">{m.valuation_runs_selected({ id: String(run.id), date: dates.format(new Date(run.captured_at)) })}</h3>
        <p class="text-sm text-muted">{m.valuation_runs_frozen_note()}</p>
        <p class="break-words text-xs text-muted">{run.inputs.profile.name} · {day(run.inputs.profile.valuation_as_of)} · {run.inputs.profile.knowledge_cutoff}</p>
        <a href={`/api/v1/investments/gains/runs/${run.id}?download=true`} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground hover:bg-control-hover">{m.valuation_runs_download()}</a>
        <details class="text-xs text-muted"><summary class="min-h-11 cursor-pointer py-3">{m.valuation_runs_fingerprints()}</summary>
          <dl class="space-y-2 break-all"><dt>{m.valuation_runs_inputs()}</dt><dd class="font-mono">{run.inputs_sha256}</dd><dt>{m.valuation_runs_output()}</dt><dd class="font-mono">{run.output_sha256}</dd></dl>
        </details>
        {#if 'rows' in run.output}
          <GainsStatement saved={run.output} />
        {:else}
          {@const output = run.output}
          <h4 class="text-sm font-semibold text-foreground">{m.investments_gains_realized_title()}</h4>
          {#if output.realized.length === 0}<p class="text-sm text-muted">{m.investments_gains_realized_empty()}</p>{/if}
          {#each output.realized_totals as total (total.cost_commodity_id)}
            <p class="text-sm text-foreground">{m.investments_gains_realized_total()}: <span class="font-mono">{amount(total.total_gain_value, total.total_gain_scale, total.cost_commodity_id)}</span></p>
          {/each}
          <div class="grid gap-3 sm:grid-cols-2">
            {#each output.realized as entry, index (index)}
              <Panel><p class="text-sm text-foreground">{m.valuation_runs_position({ instrument: String(entry.commodity_id), account: String(entry.account_id) })}<PositionSideBadge side={entry.position_side} /></p>
                <p class="text-xs text-muted">{day(entry.disposal_date)}</p>
                <dl class="space-y-1 text-sm text-foreground">
                  <dt>{m.investments_col_quantity()}</dt><dd class="font-mono">{formatQuantity(entry.quantity_value, entry.quantity_scale, locale)}</dd>
                  <dt>{m.investments_gains_col_proceeds()}</dt><dd class="font-mono">{entry.position_side === 'short' ? amount(entry.disposed_basis_value, entry.disposed_basis_scale, entry.cost_commodity_id) : amount(entry.proceeds_value, entry.proceeds_scale, entry.cost_commodity_id)}</dd>
                  <dt>{m.investments_col_cost_basis()}</dt><dd class="font-mono">{entry.position_side === 'short' ? amount(negateCoefficient(entry.proceeds_value), entry.proceeds_scale, entry.cost_commodity_id) : amount(entry.disposed_basis_value === null ? null : negateCoefficient(entry.disposed_basis_value), entry.disposed_basis_scale, entry.cost_commodity_id)}</dd>
                  <dt>{m.investments_gains_col_gain()}</dt><dd class="font-mono">{amount(entry.realized_gain_value, entry.realized_gain_scale, entry.cost_commodity_id)}</dd>
                </dl>
              </Panel>
            {/each}
          </div>
          <h4 class="text-sm font-semibold text-foreground">{m.investments_gains_unrealized_title()}</h4>
          {#if output.unrealized.length === 0}<p class="text-sm text-muted">{m.investments_gains_unrealized_empty()}</p>{/if}
          <div class="grid gap-3 sm:grid-cols-2">
            {#each output.unrealized as position (position.account_id + '_' + position.commodity_id + '_' + position.cost_commodity_id + '_' + position.position_side)}
              <Panel><p class="text-sm text-foreground">{m.valuation_runs_position({ instrument: String(position.commodity_id), account: String(position.account_id) })}<PositionSideBadge side={position.position_side} /><ObservationStatus status={position.price_status} /></p>
                <dl class="space-y-1 text-sm text-foreground">
                  <dt>{m.investments_col_quantity()}</dt><dd class="font-mono">{formatQuantity(position.quantity_value, position.quantity_scale, locale)}</dd>
                  <dt>{m.investments_col_cost_basis()}</dt><dd class="font-mono">{amount(position.remaining_cost_basis_value, position.remaining_cost_basis_scale, position.cost_commodity_id)}</dd>
                  <dt>{m.investments_gains_col_market_value()}</dt><dd class="font-mono">{amount(position.market_value_value, position.market_value_scale, position.cost_commodity_id)}</dd>
                  <dt>{m.investments_gains_col_gain()}</dt><dd class="font-mono">{amount(position.unrealized_gain_value, position.unrealized_gain_scale, position.cost_commodity_id)}</dd>
                </dl>
                {#if position.latest_price_date}<p class="text-xs text-muted">{m.valuation_gains_price_date({ date: day(position.latest_price_date) })}</p>{/if}
                {#if position.latest_price_approximate}<p class="text-xs text-muted">{m.investments_trade_price_approximate()}</p>{/if}
                {#if position.basis_knowledge === 'unknown'}<p class="text-xs text-muted">{m.investments_basis_unknown()}</p>{/if}
              </Panel>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</section>
