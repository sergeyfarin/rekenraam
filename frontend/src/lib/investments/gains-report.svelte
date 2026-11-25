<script lang="ts">
  import SaveGainsRun from './save-gains-run.svelte';
  import GainsRuns from './gains-runs.svelte';
  import { page } from '$app/state';
  let { csrfToken }: { csrfToken: string } = $props();
  let selectedRunID = $state(0);
  $effect(() => {
    const raw = page.url.searchParams.get('run_id');
    const id = Number(raw);
    selectedRunID = raw && /^[1-9]\d*$/.test(raw) && Number.isSafeInteger(id) ? id : 0;
  });
  function runSaved(id: number) { selectedRunID = id; }
  import GainsFXView from './gains-fx-view.svelte';
  import type { GainsFXInput, InvestmentGainsResponse } from '#lib/api/investments.ts';
  import AlternativeGains from './alternative-gains.svelte';
  import type { AlternativeBasisMethod } from '#lib/api/investments.ts';
  import ObservationStatus from '#lib/valuation/observation-status.svelte';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { createQuery } from '@tanstack/svelte-query';
  import CapitalReturnSummary from '#lib/investments/capital-return-summary.svelte';
  import GainsStatement from '#lib/investments/gains-statement.svelte';
  import Panel from '#lib/components/panel.svelte';
  import StatePanel from '#lib/components/state-panel.svelte';
  import {
    investmentGainsQueryOptions,
    investmentInstrumentsQueryOptions
  } from '#lib/api/investments.ts';
  import { formatMoney, joinCommodityAmount } from '#lib/money/format.ts';
  import { coefficientSign, negateCoefficient } from '#lib/money/amount.ts';
  import { formatScaledValue } from './investment-labels';
  import PositionSideBadge from './position-side-badge.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  const ownerToday = useOwnerToday();

  const locale = $derived(getLocale());

  // Date range filter (default: current calendar year)
  const currentYear = ownerToday().slice(0, 4);
  let fromDate = $state(`${currentYear}-01-01`);
  let toDate = $state(`${currentYear}-12-31`);
  let appliedFrom = $state(`${currentYear}-01-01`);
  let appliedTo = $state(`${currentYear}-12-31`);

  function applyFilter() {
    appliedFrom = fromDate;
    appliedTo = toDate;
  }

  // Unrealized valuation date (R18 operational_v1, #230). Today is sent as no
  // date so the server values its current projection at owner-local today.
  const today = ownerToday();
  let valuationDate = $state(today);
  let appliedValuationDate = $state(today);
  const valuationDateFuture = $derived(valuationDate > today);

  function applyValuationDate() {
    if (valuationDate === '' || valuationDateFuture) return;
    appliedValuationDate = valuationDate;
  }

  let reportingCurrency = $state('');
  let fxMethod = $state<GainsFXInput['fx_method']>('valuation_date_all');
  let fxMaxAge = $state(7);
  const fxInvalid = $derived(!Number.isInteger(fxMaxAge) || fxMaxAge < 1 || fxMaxAge > 365);
  const fx = $derived<GainsFXInput | undefined>(reportingCurrency && !fxInvalid ? { reporting_currency_id: Number(reportingCurrency), fx_method: fxMethod, fx_max_age_days: fxMaxAge } : undefined);
  let currencyOptions = $state.raw<InvestmentGainsResponse['currencies']>([]);
  $effect(() => { if (gainsQuery.data) currencyOptions = gainsQuery.data.currencies; });

  let jurisdictionNote = $state('');
  let appliedJurisdictionNote = $state('');
  let alternativeMethod = $state<AlternativeBasisMethod | ''>('');

  const gainsQuery = createQuery(() =>
    investmentGainsQueryOptions(
      appliedFrom || undefined,
      appliedTo || undefined,
      appliedValuationDate === today ? undefined : appliedValuationDate,
      alternativeMethod || undefined,
      fx,
      alternativeMethod === 'person_pooled_average' ? appliedJurisdictionNote || undefined : undefined
    )
  );
  const profile = $derived(gainsQuery.data?.profile);
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());

  const instrumentsByID = $derived.by(() => {
    const map = new Map<number, string>();
    for (const inst of instrumentsQuery.data?.instruments ?? []) {
      map.set(inst.commodity_id, inst.display_name);
    }
    return map;
  });

  function instrumentName(commodityID: number): string {
    return instrumentsByID.get(commodityID) ?? `#${commodityID}`;
  }

  const realized = $derived(gainsQuery.data?.realized ?? []);
  const unrealized = $derived(gainsQuery.data?.unrealized ?? []);
  // Totals are pre-aggregated by the backend, grouped by cost_commodity_id before display rounding.
  const realizedTotals = $derived(gainsQuery.data?.realized_totals ?? []);

  function formatGain(value: string, scale: number, currencyID: number): string {
    const currency = gainsQuery.data?.currencies.find((c) => c.id === currencyID);
    const amount = currency
      ? formatMoney(String(value), scale, currency.standard_scale, locale)
      : formatScaledValue(String(value), scale, locale);
    return joinCommodityAmount(currency?.code ?? `#${currencyID}`, amount);
  }

  // A position with no price and one the backend could not represent both come
  // back without a market value; only the second is worth telling the user
  // about, so the two reasons must not share the placeholder dash.
  function unavailableLabel(reason: string | undefined): string {
    if (reason === 'unrepresentable') return m.investments_gains_value_out_of_range();
    if (reason === 'conflicting_splits') return m.valuation_price_conflicting_splits();
    return m.investments_gains_no_price();
  }

  function gainClass(value: string): string {
    const sign = coefficientSign(value);
    if (sign > 0) return 'text-positive';
    if (sign < 0) return 'text-danger';
    return 'text-muted';
  }
</script>

<div class="space-y-6">
  <SaveGainsRun request={{ report_type: 'gains', from: appliedFrom || undefined, to: appliedTo || undefined, valuation_as_of: appliedValuationDate }} {csrfToken} onSaved={runSaved} />
  <GainsRuns bind:selectedID={selectedRunID} />
  <p class="text-sm text-muted">{m.valuation_holdings_policy()}</p>
  <section class="space-y-3" data-testid="alternative-gains">
    <label class="flex flex-wrap items-center gap-2 text-sm text-foreground">
      {m.valuation_alternative_method()}
      <select bind:value={alternativeMethod} class="max-w-full rounded border border-border bg-control px-2 py-1 text-foreground">
        <option value="">{m.valuation_alternative_none()}</option>
        <option value="fifo">{m.transactions_investment_replace_method_fifo()}</option>
        <option value="lifo">{m.transactions_investment_replace_method_lifo()}</option>
        <option value="average_cost">{m.transactions_investment_replace_method_average()}</option>
        <option value="person_pooled_average">{m.valuation_person_pool_method()}</option>
      </select>
    </label>
    {#if alternativeMethod === 'person_pooled_average'}
      <form class="flex flex-wrap items-end gap-2 text-sm" onsubmit={(event) => { event.preventDefault(); appliedJurisdictionNote = jurisdictionNote.trim(); }}>
        <label class="grid gap-1 text-muted">{m.valuation_person_pool_note()}
          <input bind:value={jurisdictionNote} maxlength="500" class="rounded border border-border bg-control px-2 py-1 text-foreground" />
        </label>
        <button type="submit" class="rounded border border-border bg-control px-3 py-1 text-foreground hover:bg-control-hover">{m.investments_gains_filter_apply()}</button>
      </form>
    {/if}
    {#if alternativeMethod}
      {#if gainsQuery.isPending}
        <Panel><p class="text-sm text-muted">{m.investments_gains_loading()}</p></Panel>
      {:else if gainsQuery.isError}
        <Panel><p class="text-sm text-danger">{m.investments_gains_error()}</p></Panel>
      {:else if gainsQuery.data?.alternative}
        <AlternativeGains data={gainsQuery.data} {instrumentName} {formatGain} />
      {/if}
    {/if}
  </section>
  <section class="space-y-3" aria-label={m.valuation_fx_title()} data-testid="gains-fx-controls">
    <h2 class="text-sm font-semibold text-foreground">{m.valuation_fx_title()}</h2>
    <div class="flex flex-wrap items-end gap-3 text-sm">
      <label class="grid gap-1 text-muted">{m.valuation_fx_currency()}
        <select bind:value={reportingCurrency} class="rounded border border-border bg-control px-2 py-1 text-foreground">
          <option value="">{m.valuation_fx_original()}</option>
          {#each currencyOptions as currency (currency.id)}<option value={String(currency.id)}>{currency.code}</option>{/each}
        </select>
      </label>
      <label class="grid w-full min-w-0 gap-1 text-muted sm:w-auto">{m.valuation_fx_method()}
        <select bind:value={fxMethod} class="w-full min-w-0 max-w-full rounded border border-border bg-control px-2 py-1 text-foreground">
          <option value="valuation_date_all">{m.valuation_fx_valuation()}</option>
          <option value="historic_basis_valuation_close">{m.valuation_fx_historic()}</option>
        </select>
      </label>
      <label class="grid gap-1 text-muted">{m.valuation_fx_age()}
        <input type="number" min="1" max="365" step="1" bind:value={fxMaxAge} aria-invalid={fxInvalid} class="w-24 rounded border border-border bg-control px-2 py-1 text-foreground" />
      </label>
    </div>
    {#if fxInvalid}<p class="text-sm text-danger" role="alert">{m.valuation_fx_age_invalid()}</p>{/if}
    <p class="text-xs text-muted">{fxMethod === 'valuation_date_all' ? m.valuation_fx_valuation_note() : m.valuation_fx_historic_note()}</p>
    {#if fx}
      {#if gainsQuery.isPending}<Panel><p role="status" class="text-sm text-muted">{m.investments_gains_loading()}</p></Panel>
      {:else if gainsQuery.isError}<Panel><p role="alert" class="text-sm text-danger">{m.investments_gains_error()}</p></Panel>
      {:else if gainsQuery.data?.reporting}
        {@const reporting = gainsQuery.data.reporting}
        <GainsFXView profile={reporting.profile} {formatGain} figures={[
          { label: m.investments_gains_realized_total(), figure: reporting.realized_total },
          { label: m.investments_gains_unrealized_title(), figure: reporting.unrealized_total },
          ...reporting.realized.map((figure, i) => ({ label: `${realized[i].disposal_date} · ${instrumentName(realized[i].commodity_id)}`, figure })),
          ...reporting.unrealized.map((figure, i) => ({ label: instrumentName(unrealized[i].commodity_id), figure }))
        ]} />
      {/if}
    {/if}
  </section>
  <!-- Unrealized gains -->
  <section>
    <div class="mb-3 flex flex-wrap items-center gap-3">
      <h2 class="text-sm font-semibold text-foreground">{m.investments_gains_unrealized_title()}</h2>
      <form
        class="flex flex-wrap items-center gap-2 text-sm"
        onsubmit={(event) => {
          event.preventDefault();
          applyValuationDate();
        }}
      >
        <label class="flex items-center gap-1 text-muted">
          {m.valuation_gains_date()}
          <input
            type="date"
            bind:value={valuationDate}
            max={today}
            aria-invalid={valuationDateFuture}
            aria-describedby={valuationDateFuture ? 'gains-valuation-date-error' : undefined}
            class="rounded border border-border bg-control px-2 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-foreground/30"
          />
        </label>
        <button
          type="submit"
          disabled={valuationDateFuture || valuationDate === ''}
          class="rounded-(--radius-control) border border-border bg-control px-3 py-1 text-xs font-medium text-foreground hover:bg-control-hover disabled:opacity-50"
        >
          {m.valuation_gains_apply()}
        </button>
      </form>
    </div>
    {#if valuationDateFuture}
      <p id="gains-valuation-date-error" class="mb-3 text-sm text-danger">{m.valuation_gains_date_future()}</p>
    {/if}
    {#if profile}
      <p class="mb-3 text-sm text-muted" data-testid="gains-profile">{m.valuation_gains_profile({ date: profile.valuation_as_of })}</p>
    {/if}
    {#if gainsQuery.isPending}
      <Panel>
        <p class="text-sm text-muted">{m.investments_gains_loading()}</p>
      </Panel>
    {:else if gainsQuery.isError}
      <Panel>
        <p class="text-sm text-danger">{m.investments_gains_error()}</p>
      </Panel>
    {:else if unrealized.length === 0}
      <StatePanel title={m.investments_gains_unrealized_empty()} copy="" />
    {:else}
      <Panel padding="none">
        <!-- svelte-ignore a11y_no_noninteractive_tabindex (keyboard users must be able to scroll the bounded table region) -->
        <div class="overflow-x-auto" role="region" aria-label={m.investments_gains_unrealized_title()} tabindex="0">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-border bg-surface-strong/40">
                <th class="py-3 pl-5 pr-3 text-left font-semibold text-foreground">{m.investments_col_instrument()}</th>
                <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_col_quantity()}</th>
                <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_col_cost_basis()}</th>
                <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_gains_col_market_value()}</th>
                <th class="py-3 pl-3 pr-5 text-right font-semibold text-foreground">{m.investments_gains_col_gain()}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border">
              {#each unrealized as pos (pos.account_id + '_' + pos.commodity_id + '_' + pos.cost_commodity_id + '_' + pos.position_side)}
                <tr class="hover:bg-surface-strong/20">
                  <td class="py-3 pl-5 pr-3 font-medium text-foreground">{instrumentName(pos.commodity_id)}<PositionSideBadge side={pos.position_side} /><ObservationStatus status={pos.price_status} /></td>
                  <td class="px-3 py-3 text-right font-mono text-foreground">
                    {formatScaledValue(pos.quantity_value, pos.quantity_scale, locale)}
                  </td>
                  <td class="px-3 py-3 text-right font-mono text-muted">
                    {pos.remaining_cost_basis_value !== null && pos.remaining_cost_basis_scale !== null ? formatGain(pos.remaining_cost_basis_value, pos.remaining_cost_basis_scale, pos.cost_commodity_id) : m.investments_basis_unknown()}
                  </td>
                  <td class="px-3 py-3 text-right font-mono text-foreground">
                    {#if pos.market_value_value !== undefined && pos.market_value_scale !== undefined}
                      {formatGain(pos.market_value_value, pos.market_value_scale, pos.cost_commodity_id)}
                      {#if pos.latest_price_date}<span class="block text-xs text-muted">{m.valuation_gains_price_date({ date: pos.latest_price_date })}</span>{/if}
                      {#if pos.latest_price_approximate}<span class="block text-xs text-muted">{m.investments_trade_price_approximate()}</span>{/if}
                    {:else}
                      <span class="text-muted">{unavailableLabel(pos.valuation_unavailable)}</span>
                    {/if}
                  </td>
                  <td class="py-3 pl-3 pr-5 text-right font-mono">
                    {#if pos.unrealized_gain_value !== undefined && pos.unrealized_gain_scale !== undefined}
                      <span class={gainClass(pos.unrealized_gain_value)}>
                        {formatGain(pos.unrealized_gain_value, pos.unrealized_gain_scale, pos.cost_commodity_id)}
                      </span>
                    {:else}
                      <span class="text-muted">{pos.gain_unavailable === 'unknown_basis' ? m.investments_basis_unknown() : unavailableLabel(pos.valuation_unavailable)}</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      </Panel>
    {/if}
  </section>

  <GainsStatement {fx} {csrfToken} onRunSaved={runSaved} />

  <!-- Realized gains -->
  <section>
    <div class="mb-3 flex flex-wrap items-center gap-3">
      <h2 class="text-sm font-semibold text-foreground">{m.investments_gains_realized_title()}</h2>
      <div class="flex flex-wrap items-center gap-2 text-sm">
        <label class="flex items-center gap-1 text-muted">
          {m.investments_gains_from()}
          <input
            type="date"
            bind:value={fromDate}
            class="rounded border border-border bg-control px-2 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-foreground/30"
          />
        </label>
        <label class="flex items-center gap-1 text-muted">
          {m.investments_gains_to()}
          <input
            type="date"
            bind:value={toDate}
            class="rounded border border-border bg-control px-2 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-foreground/30"
          />
        </label>
        <button
          type="button"
          onclick={applyFilter}
          class="rounded-(--radius-control) border border-border bg-control px-3 py-1 text-xs font-medium text-foreground hover:bg-control-hover"
        >
          {m.investments_gains_filter_apply()}
        </button>
      </div>
    </div>

    {#if gainsQuery.isPending}
      <Panel>
        <p class="text-sm text-muted">{m.investments_gains_loading()}</p>
      </Panel>
    {:else if gainsQuery.isError}
      <Panel>
        <p class="text-sm text-danger">{m.investments_gains_error()}</p>
      </Panel>
    {:else if realized.length === 0}
      <StatePanel title={m.investments_gains_realized_empty()} copy="" />
    {:else}
      <Panel padding="none">
        <!-- svelte-ignore a11y_no_noninteractive_tabindex (keyboard users must be able to scroll the bounded table region) -->
        <div class="overflow-x-auto" role="region" aria-label={m.investments_gains_realized_title()} tabindex="0">
          <table class="min-w-full text-sm">
            <thead>
              <tr class="border-b border-border bg-surface-strong/40">
                <th class="py-3 pl-5 pr-3 text-left font-semibold text-foreground">{m.investments_gains_col_date()}</th>
                <th class="px-3 py-3 text-left font-semibold text-foreground">{m.investments_col_instrument()}</th>
                <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_col_quantity()}</th>
                <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_gains_col_proceeds()}</th>
                <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_gains_col_disposed_basis()}</th>
                <th class="py-3 pl-3 pr-5 text-right font-semibold text-foreground">{m.investments_gains_col_gain()}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border">
              {#each realized as entry}
                <tr class="hover:bg-surface-strong/20">
                  <td class="py-3 pl-5 pr-3 font-mono text-xs text-muted">{entry.disposal_date}</td>
                  <td class="px-3 py-3 text-foreground">{instrumentName(entry.commodity_id)}<PositionSideBadge side={entry.position_side} /></td>
                  <td class="px-3 py-3 text-right font-mono text-foreground">
                    {formatScaledValue(entry.quantity_value, entry.quantity_scale, locale)}
                  </td>
                  {#if entry.position_side === 'short' && entry.disposed_basis_value !== null && entry.disposed_basis_scale !== null}
                    <!-- A cover's proceeds are what the short received when it
                         opened; its cost is what covering paid (#173). -->
                    <td class="px-3 py-3 text-right font-mono text-foreground">
                      {formatGain(entry.disposed_basis_value, entry.disposed_basis_scale, entry.cost_commodity_id)}
                    </td>
                    <td class="px-3 py-3 text-right font-mono text-muted">
                      {formatGain(negateCoefficient(entry.proceeds_value), entry.proceeds_scale, entry.cost_commodity_id)}
                    </td>
                  {:else}
                    <td class="px-3 py-3 text-right font-mono text-foreground">
                      {formatGain(entry.proceeds_value, entry.proceeds_scale, entry.cost_commodity_id)}
                    </td>
                    <td class="px-3 py-3 text-right font-mono text-muted">
                      {#if entry.disposed_basis_value !== null && entry.disposed_basis_scale !== null}
                        {formatGain(negateCoefficient(entry.disposed_basis_value), entry.disposed_basis_scale, entry.cost_commodity_id)}
                      {:else}
                        {m.investments_basis_unknown()}
                      {/if}
                    </td>
                  {/if}
                  <td class="py-3 pl-3 pr-5 text-right font-mono">
                    {#if entry.realized_gain_value !== null && entry.realized_gain_scale !== null}
                      <span class={gainClass(entry.realized_gain_value)}>
                        {formatGain(entry.realized_gain_value, entry.realized_gain_scale, entry.cost_commodity_id)}
                      </span>
                    {:else}
                      <span class="text-muted">{m.investments_gain_unresolved()}</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
            <tfoot>
              {#each realizedTotals as tot (tot.cost_commodity_id)}
                <tr class="border-t-2 border-border bg-surface-strong/30">
                  <td colspan="5" class="py-3 pl-5 pr-3 text-sm font-semibold text-foreground">
                    {m.investments_gains_realized_total()}
                  </td>
                  <td class="py-3 pl-3 pr-5 text-right font-mono font-semibold">
                    {#if tot.total_gain_value !== null && tot.total_gain_scale !== null}
                      <span class={gainClass(tot.total_gain_value)}>
                        {formatGain(tot.total_gain_value, tot.total_gain_scale, tot.cost_commodity_id)}
                      </span>
                    {:else}
                      <span class="text-muted">{m.investments_gains_total_unresolved({ count: tot.unresolved_count })}</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tfoot>
          </table>
        </div>
      </Panel>
    {/if}
  </section>
  <section class="space-y-3" aria-label={m.investments_roc_summary_title()}>
    <h2 class="text-sm font-semibold text-foreground">{m.investments_roc_summary_title()}</h2>
    <p class="text-xs text-muted">{m.investments_roc_period_note()}</p>
    {#if gainsQuery.isPending}
      <Panel><p class="text-sm text-muted" role="status">{m.investments_gains_loading()}</p></Panel>
    {:else if gainsQuery.isError}
      <Panel><p class="text-sm text-danger" role="alert">{m.investments_gains_error()}</p></Panel>
    {:else if !gainsQuery.data?.capital_returns?.length}
      <StatePanel title={m.investments_roc_empty()} copy="" />
    {:else}
      <div class="grid gap-3 md:grid-cols-2">
        {#each gainsQuery.data.capital_returns as entry (entry.operation_id)}
          <CapitalReturnSummary {entry} />
        {/each}
      </div>
    {/if}
  </section>
</div>
