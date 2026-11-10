<script lang="ts">
  import { publishPanelHeight } from '#lib/components/sticky-panel.ts';
  import ObservationStatus from '#lib/valuation/observation-status.svelte';
  import { page } from '$app/state';
  import { createQuery } from '@tanstack/svelte-query';
  import TrendingUp from '@lucide/svelte/icons/trending-up';
  import X from '@lucide/svelte/icons/x';
  import Panel from '#lib/components/panel.svelte';
  import PageHeader from '#lib/components/page-header.svelte';
  import { phoneModal } from '#lib/components/modal-focus.ts';
  import StatePanel from '#lib/components/state-panel.svelte';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import {
    investmentPositionsQueryOptions,
    investmentLotsQueryOptions,
    investmentInstrumentsQueryOptions,
    investmentEventSuggestionsQueryOptions,
    type InvestmentLotOrigin,
    type InvestmentPositionResponse
  } from '#lib/api/investments.ts';
  import SplitActivity from '#lib/investments/split-activity.svelte';
  import { addTransaction } from '#lib/transactions/add-transaction.svelte.ts';
  import PositionSideBadge from '#lib/investments/position-side-badge.svelte';
  import EventSuggestions from '#lib/investments/event-suggestions.svelte';
  import { parseISO } from 'date-fns';
  import { formatScaledValue } from './investment-labels';
  import { coefficientSign, negateCoefficient } from '#lib/money/amount.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  const locale = $derived(getLocale());
  const dateFormatter = $derived(
    new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'short', day: 'numeric' })
  );

  function formatDate(iso: string): string {
    return dateFormatter.format(parseISO(iso));
  }

  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const csrfToken = $derived(sessionQuery.data?.csrf_token ?? '');

  const positionsQuery = createQuery(() => investmentPositionsQueryOptions());
  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());

  let selectedPosition = $state.raw<InvestmentPositionResponse | null>(null);

  const lotsQuery = createQuery(() => ({
    ...investmentLotsQueryOptions(
      selectedPosition?.account_id,
      selectedPosition?.commodity_id
    ),
    enabled: selectedPosition !== null
  }));

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

  function selectPosition(pos: InvestmentPositionResponse) {
    selectedPosition = pos;
  }

  function closeDetail() {
    selectedPosition = null;
  }

  // Where a transferred or exchanged lot came from (#178). opened_on is the
  // day it arrived here; the original acquisition date travels with it.
  function lotOriginLine(origin: InvestmentLotOrigin, openedOn: string): string {
    if (origin.transfer_kind === 'exchange' && origin.ratio_numerator !== null && origin.ratio_denominator !== null) {
      return m.investments_lot_exchanged_from({
        instrument: instrumentName(origin.source_commodity_id), ratioNew: String(origin.ratio_numerator),
        ratioOld: String(origin.ratio_denominator), date: formatDate(openedOn)
      });
    }
    if (origin.transfer_kind === 'spin_off' && origin.ratio_numerator !== null && origin.ratio_denominator !== null) {
      return m.investments_lot_spun_off_from({
        instrument: instrumentName(origin.source_commodity_id), ratioNew: String(origin.ratio_numerator),
        ratioOld: String(origin.ratio_denominator), date: formatDate(openedOn)
      });
    }
    return m.investments_lot_transferred_in({ date: formatDate(openedOn) });
  }

  function lotStatusLabel(status: string): string {
    switch (status) {
      case 'open':
        return m.investments_lot_status_open();
      case 'closed':
        return m.investments_lot_status_closed();
      default:
        return status;
    }
  }

  const isLoading = $derived(positionsQuery.isPending || instrumentsQuery.isPending);
  const isError = $derived(positionsQuery.isError || instrumentsQuery.isError);
  const positions = $derived(positionsQuery.data?.positions ?? []);
  const openPositions = $derived(positions.filter((p) => coefficientSign(p.quantity_value) !== 0));
  const selectedLots = $derived(
    (lotsQuery.data?.lots ?? []).filter((lot) => lot.cost_commodity_id === selectedPosition?.cost_commodity_id &&
      lot.position_side === selectedPosition?.position_side)
  );
  // Position identity includes cost currency and side (validate-and-ship #29/#30).
  function positionKey(pos: InvestmentPositionResponse): string {
    return `${pos.account_id}_${pos.commodity_id}_${pos.cost_commodity_id}_${pos.position_side}`;
  }
  // A short holding owes its units: show the signed exposure.
  function signedQuantity(pos: InvestmentPositionResponse): string {
    return pos.position_side === 'short' ? negateCoefficient(pos.quantity_value) : pos.quantity_value;
  }

  // In-page views of the Overview. Instruments and Gains are their own
  // routes (#436); these stay views, read from the URL on every navigation so a
  // link to another view of this page switches it.
  type Tab = 'portfolio' | 'splits' | 'suggestions';
  const activeTab = $derived.by<Tab>(() => {
    const tab = page.url.searchParams.get('tab');
    return tab === 'splits' || tab === 'suggestions' ? tab : 'portfolio';
  });

  function tabHref(tab: Tab): string {
    return tab === 'portfolio' ? '/app/investments' : `/app/investments?tab=${tab}`;
  }

  // Badge: count pending suggestions
  const suggestionsQuery = createQuery(() => ({
    ...investmentEventSuggestionsQueryOptions(),
    // Don't throw; silently omit badge on error
    throwOnError: false
  }));
  const pendingCount = $derived(
    (suggestionsQuery.data?.suggestions ?? []).filter((s) => s.status === 'suggested').length
  );
</script>

<div>
  <PageHeader title={m.investments_nav()}>
    {#snippet actions()}
      <button type="button" onclick={() => addTransaction({ tab: 'investment' })} class="min-h-11 rounded-(--radius-control) bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground">{m.transactions_form_create_title()}</button>
    {/snippet}
  </PageHeader>
  <div>
    <p class="mb-3 text-sm text-muted"><a href="https://www.coingecko.com/en/api" target="_blank" rel="noreferrer" class="underline">{m.quotes_attribution()}</a></p>
    <!-- View navigation -->
    <nav aria-label={m.investments_title()} class="mb-5 flex items-center gap-1 overflow-x-auto border-b border-border">
      {#each (['portfolio', 'splits', 'suggestions'] as const) as tab (tab)}
        <a
          href={tabHref(tab)}
          aria-current={activeTab === tab ? 'page' : undefined}
          class="relative -mb-px flex min-h-11 shrink-0 items-center gap-1.5 px-4 text-sm font-medium transition
            {activeTab === tab
              ? 'border-b-2 border-foreground text-foreground'
              : 'text-muted hover:text-foreground'}"
        >
          {#if tab === 'portfolio'}{m.investments_tab_portfolio()}
          {:else if tab === 'splits'}{m.investments_split_history_title()}
          {:else}{m.investments_tab_suggestions()}
          {/if}
          {#if tab === 'suggestions' && pendingCount > 0}
            <span class="flex h-4.5 min-w-4.5 items-center justify-center rounded-full bg-accent px-1 text-[10px] font-semibold text-accent-foreground">
              {pendingCount}
            </span>
          {/if}
        </a>
      {/each}
    </nav>

    <!-- Portfolio tab action buttons -->
    {#if activeTab === 'portfolio'}

    {#if isLoading}
      <Panel>
        <p class="text-sm text-muted">{m.investments_loading()}</p>
      </Panel>
    {:else if isError}
      <StatePanel
        alert
        title={m.investments_error_title()}
        copy={m.investments_error_copy()}
      />
    {:else if openPositions.length === 0}
      <StatePanel
        title={m.investments_empty_title()}
        copy={m.investments_empty_copy()}
      />
    {:else}
      <div class="lg:grid lg:grid-cols-[minmax(0,1fr)_24rem] lg:gap-6">
        <!-- Positions table -->
        <div class="min-w-0">
          <Panel padding="none">
            <div class="overflow-x-auto">
              <table class="min-w-full text-sm">
                <caption class="p-3 text-left text-sm text-muted">{m.valuation_holdings_policy()}</caption>
                <thead>
                  <tr class="border-b border-border bg-surface-strong/40">
                    <th class="py-3 pl-5 pr-3 text-left font-semibold text-foreground">{m.investments_col_instrument()}</th>
                    <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_col_quantity()}</th>
                    <th class="px-3 py-3 text-right font-semibold text-foreground">{m.investments_col_cost_basis()}</th>
                    <th class="py-3 pl-3 pr-5 text-right font-semibold text-foreground">{m.investments_col_latest_price()}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-border">
                  {#each openPositions as pos (positionKey(pos))}
                    <tr
                      class={`cursor-pointer transition hover:bg-surface-strong/30 ${selectedPosition && positionKey(selectedPosition) === positionKey(pos) ? 'bg-surface-strong/50' : ''}`}
                      onclick={() => selectPosition(pos)}
                      role="button"
                      tabindex="0"
                      onkeydown={(e) => {
                        // Prevented, or the key's activation lands on the
                        // panel control that now has focus and closes it.
                        if (e.key !== 'Enter' && e.key !== ' ') return;
                        e.preventDefault();
                        selectPosition(pos);
                      }}
                    >
                      <td class="py-3 pl-5 pr-3">
                        <div class="flex items-center gap-2">
                          <TrendingUp size={14} class="shrink-0 text-muted" aria-hidden="true" />
                          <span class="font-medium text-foreground">{instrumentName(pos.commodity_id)}</span>
                          <ObservationStatus status={pos.price_status} />
                          <PositionSideBadge side={pos.position_side} />
                          <span class="sr-only">
                            {m.investments_row_aria({ name: instrumentName(pos.commodity_id) })}
                          </span>
                        </div>
                      </td>
                      <td class="px-3 py-3 text-right font-mono text-foreground">
                        {formatScaledValue(signedQuantity(pos), pos.quantity_scale, locale)}
                      </td>
                      <td class="px-3 py-3 text-right font-mono text-muted">
                        {pos.remaining_cost_basis_value !== null && pos.remaining_cost_basis_scale !== null ? formatScaledValue(pos.remaining_cost_basis_value, pos.remaining_cost_basis_scale, locale) : m.investments_basis_unknown()}
                      </td>
                      <td class="py-3 pl-3 pr-5 text-right">
                        {#if pos.latest_price_value !== undefined && pos.latest_price_scale !== undefined}
                          <span class="font-mono text-foreground">
                            {formatScaledValue(pos.latest_price_value, pos.latest_price_scale, locale)}
                          </span>
                          {#if pos.latest_price_date}
                            <span class="ml-1 text-xs text-muted">{formatDate(pos.latest_price_date)}</span>
                          {/if}
                          {#if pos.latest_price_approximate}
                            <span class="ml-1 text-xs text-muted">{m.investments_trade_price_approximate()}</span>
                          {/if}
                        {:else}
                          <span class="text-muted">—</span>
                        {/if}
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          </Panel>
        </div>

        <!-- Lot detail panel -->
        {#if selectedPosition}
          <!-- Mobile: a full-screen modal overlay (#342). Desktop: the right-hand column. -->
          <aside
            class="fixed inset-0 z-30 overflow-y-auto bg-surface outline-none lg:relative lg:inset-auto lg:z-auto lg:overflow-visible"
            aria-label={m.investments_lots_detail_label()}
            {@attach phoneModal({ onCancel: closeDetail })}
          >
            <div
              class="fixed inset-0 bg-background/50 lg:hidden"
              role="presentation"
              onclick={closeDetail}
            ></div>

            <div class="relative lg:side-panel-sticky" {@attach publishPanelHeight}>
              <Panel>
                <div class="mb-4 flex items-center justify-between">
                  <h2 class="text-base font-semibold text-foreground">
                    {instrumentName(selectedPosition.commodity_id)} — {m.investments_lots_title()}
                  </h2>
                  <button
                    type="button"
                    onclick={closeDetail}
                    class="rounded p-1 text-muted transition hover:text-foreground"
                    aria-label={m.investments_lots_close()}
                  >
                    <X size={16} aria-hidden="true" />
                  </button>
                </div>

                <a
                  href={`/app/investments/instruments/${selectedPosition.commodity_id}`}
                  class="mb-3 inline-flex min-h-11 items-center text-sm font-semibold underline underline-offset-4"
                >
                  {m.investments_instrument_view_open()}
                </a>

                {#if lotsQuery.isPending}
                  <p class="text-sm text-muted">{m.investments_lots_loading()}</p>
                {:else if lotsQuery.isError}
                  <p class="text-sm text-danger">{m.investments_lots_error()}</p>
                {:else if selectedLots.length === 0}
                  <p class="text-sm text-muted">{m.investments_lots_empty()}</p>
                {:else}
                  <div class="space-y-3">
                    {#each selectedLots as lot (lot.id)}
                      <div class="rounded-(--radius-panel) border border-border p-3 text-sm">
                        <div class="flex items-center justify-between gap-2">
                          <span class="font-medium text-foreground">{formatDate(lot.opened_on)}</span>
                          <span
                            class="rounded-full px-2 py-0.5 text-xs font-medium {lot.status === 'open' ? 'bg-positive-soft text-positive' : 'bg-surface-strong text-foreground'}"
                          >
                            {lotStatusLabel(lot.status)}
                          </span>
                        </div>
                        <div class="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 text-xs wrap-anywhere">
                          <span class="text-muted">{m.investments_lot_remaining_qty()}</span>
                          <span class="text-right font-mono text-foreground">
                            {formatScaledValue(lot.remaining_quantity_value, lot.remaining_quantity_scale, locale)}
                          </span>
                          <span class="text-muted">{selectedPosition.position_side === 'short' ? m.investments_short_opening_proceeds() : m.investments_lot_cost_basis()}</span>
                          <span class="text-right font-mono text-foreground">
                            {lot.remaining_cost_basis_value !== null && lot.remaining_cost_basis_scale !== null ? formatScaledValue(lot.remaining_cost_basis_value, lot.remaining_cost_basis_scale, locale) : m.investments_basis_unknown()}
                          </span>
                          <span class="text-muted">{m.investments_lot_original_qty()}</span>
                          <span class="text-right font-mono text-muted">
                            {formatScaledValue(lot.quantity_value, lot.quantity_scale, locale)}
                          </span>
                          {#if lot.origin}
                            <span class="text-muted">{m.investments_lot_original_acquired()}</span>
                            <span class="text-right text-foreground">
                              {lot.origin.original_acquired_on ? formatDate(lot.origin.original_acquired_on) : m.investments_lot_original_acquired_unknown()}
                            </span>
                          {/if}
                        </div>
                        {#if lot.origin}
                          <p class="mt-2 text-xs text-muted">{lotOriginLine(lot.origin, lot.opened_on)}</p>
                        {/if}
                      </div>
                    {/each}
                  </div>
                {/if}
              </Panel>
            </div>
          </aside>
        {/if}
      </div>
    {/if}
    {/if}

    {#if activeTab === 'splits'}<SplitActivity {csrfToken}/>{/if}

    <!-- Suggestions tab -->
    {#if activeTab === 'suggestions'}
      <EventSuggestions {csrfToken} />
    {/if}
  </div>
</div>
