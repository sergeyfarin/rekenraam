<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import TrendingUp from '@lucide/svelte/icons/trending-up';
  import X from '@lucide/svelte/icons/x';
  import Plus from '@lucide/svelte/icons/plus';
  import Panel from '#lib/components/panel.svelte';
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
  import BuyForm from '#lib/investments/buy-form.svelte';
  import SellForm from '#lib/investments/sell-form.svelte';
  import DividendForm from '#lib/investments/dividend-form.svelte';
  import ExternalTransferInForm from '#lib/investments/external-transfer-in-form.svelte';
  import InternalTransferForm from '#lib/investments/internal-transfer-form.svelte';
  import ExternalTransferOutForm from '#lib/investments/external-transfer-out-form.svelte';
  import CapitalReturnForm from '#lib/investments/capital-return-form.svelte';
  import SplitForm from '#lib/investments/split-form.svelte';
  import ShareExchangeForm from '#lib/investments/share-exchange-form.svelte';
  import SpinOffForm from '#lib/investments/spin-off-form.svelte';
  import ShortTradeForm from '#lib/investments/short-trade-form.svelte';
  import PositionSideBadge from '#lib/investments/position-side-badge.svelte';
  import GainsReport from '#lib/investments/gains-report.svelte';
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

  let selectedPosition = $state<InvestmentPositionResponse | null>(null);

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

  // Trade form modal
  type TradeModal = 'buy' | 'sell' | 'short-open' | 'short-cover' | 'dividend' | 'reinvested' | 'external-transfer-in' | 'external-transfer-out' | 'internal-transfer' | 'split' | 'share-exchange' | 'spin-off' | 'capital-return' | null;
  let activeModal = $state<TradeModal>(null);

  function openModal(modal: TradeModal) {
    activeModal = modal;
  }

  function closeModal() {
    activeModal = null;
  }

  function onTradeSaved() {
    closeModal();
  }

  // Tab navigation
  type Tab = 'portfolio' | 'gains' | 'suggestions';
  let activeTab = $state<Tab>('portfolio');

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
  <div class="p-4 sm:p-6 lg:p-8">
    <!-- Tab navigation -->
    <div class="mb-5 flex items-center gap-1 border-b border-border">
      {#each (['portfolio', 'gains', 'suggestions'] as const) as tab}
        <button
          type="button"
          onclick={() => { activeTab = tab; }}
          class="relative -mb-px flex items-center gap-1.5 px-4 py-2.5 text-sm font-medium transition
            {activeTab === tab
              ? 'border-b-2 border-foreground text-foreground'
              : 'text-muted hover:text-foreground'}"
        >
          {#if tab === 'portfolio'}{m.investments_tab_portfolio()}
          {:else if tab === 'gains'}{m.investments_tab_gains()}
          {:else}{m.investments_tab_suggestions()}
          {/if}
          {#if tab === 'suggestions' && pendingCount > 0}
            <span class="flex h-4.5 min-w-4.5 items-center justify-center rounded-full bg-foreground px-1 text-[10px] font-semibold text-background">
              {pendingCount}
            </span>
          {/if}
        </button>
      {/each}
    </div>

    <!-- Portfolio tab action buttons -->
    {#if activeTab === 'portfolio'}
    <!-- Action buttons -->
    <div class="mb-4 flex flex-wrap gap-2">
      <button
        type="button"
        onclick={() => openModal('buy')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-3 py-2 text-sm font-semibold text-background transition hover:opacity-90"
      >
        <Plus size={14} aria-hidden="true" />
        {m.investments_record_buy()}
      </button>
      <button
        type="button"
        onclick={() => openModal('sell')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      >
        {m.investments_record_sell()}
      </button>
      <button
        type="button"
        onclick={() => openModal('short-open')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      >
        {m.investments_record_short_sale()}
      </button>
      <button
        type="button"
        onclick={() => openModal('short-cover')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      >
        {m.investments_record_short_cover()}
      </button>
      <button
        type="button"
        onclick={() => openModal('dividend')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      >
        {m.investments_record_dividend()}
      </button>
      <button
        type="button"
        onclick={() => openModal('reinvested')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      >
        {m.investments_record_reinvested()}
      </button>
      <button
        type="button"
        onclick={() => openModal('external-transfer-in')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      >
        {m.investments_record_external_transfer_in()}
      </button>
      <button
        type="button"
        onclick={() => openModal('internal-transfer')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      >
        {m.investments_record_internal_transfer()}
      </button>
      <button
        type="button"
        onclick={() => openModal('external-transfer-out')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      >
        {m.investments_record_external_transfer_out()}
      </button>
      <button
        type="button"
        onclick={() => openModal('split')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      >
        {m.investments_record_split()}
      </button>
      <button
        type="button"
        onclick={() => openModal('share-exchange')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      >
        {m.investments_record_share_exchange()}
      </button>
      <button
        type="button"
        onclick={() => openModal('spin-off')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      >
        {m.investments_record_spin_off()}
      </button>
      <button
        type="button"
        onclick={() => openModal('capital-return')}
        class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      >
        {m.investments_record_capital_return()}
      </button>
    </div>

    {#if isLoading}
      <Panel>
        <p class="text-sm text-muted">{m.investments_loading()}</p>
      </Panel>
    {:else if isError}
      <StatePanel
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
                      onkeydown={(e) => (e.key === 'Enter' || e.key === ' ') && selectPosition(pos)}
                      aria-label={m.investments_row_aria({ name: instrumentName(pos.commodity_id) })}
                    >
                      <td class="py-3 pl-5 pr-3">
                        <div class="flex items-center gap-2">
                          <TrendingUp size={14} class="shrink-0 text-muted" aria-hidden="true" />
                          <span class="font-medium text-foreground">{instrumentName(pos.commodity_id)}</span>
                          <PositionSideBadge side={pos.position_side} />
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
          <aside
            class="fixed inset-0 z-30 overflow-y-auto bg-surface lg:relative lg:inset-auto lg:z-auto lg:overflow-visible"
            aria-label={m.investments_lots_detail_label()}
          >
            <div
              class="fixed inset-0 bg-background/50 lg:hidden"
              role="presentation"
              onclick={closeDetail}
            ></div>

            <div class="relative lg:sticky lg:top-4">
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

                {#if lotsQuery.isPending}
                  <p class="text-sm text-muted">{m.investments_lots_loading()}</p>
                {:else if lotsQuery.isError}
                  <p class="text-sm text-destructive">{m.investments_lots_error()}</p>
                {:else if selectedLots.length === 0}
                  <p class="text-sm text-muted">{m.investments_lots_empty()}</p>
                {:else}
                  <div class="space-y-3">
                    {#each selectedLots as lot (lot.id)}
                      <div class="rounded-(--radius-panel) border border-border p-3 text-sm">
                        <div class="flex items-center justify-between gap-2">
                          <span class="font-medium text-foreground">{formatDate(lot.opened_on)}</span>
                          <span
                            class="rounded-full px-2 py-0.5 text-xs font-medium {lot.status === 'open' ? 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400' : 'bg-muted/30 text-muted'}"
                          >
                            {lotStatusLabel(lot.status)}
                          </span>
                        </div>
                        <div class="mt-2 grid grid-cols-2 gap-x-3 gap-y-1 text-xs">
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

    <!-- Gains tab -->
    {#if activeTab === 'gains'}
      <GainsReport />
    {/if}

    <!-- Suggestions tab -->
    {#if activeTab === 'suggestions'}
      <EventSuggestions {csrfToken} />
    {/if}
  </div>
</div>

<!-- Trade form modal -->
{#if activeModal !== null}
  <!-- Backdrop -->
  <div
    class="fixed inset-0 z-40 bg-background/60 backdrop-blur-sm"
    role="presentation"
    onclick={closeModal}
  ></div>

  <!-- Modal panel -->
  <div
    class="fixed inset-x-4 bottom-0 z-50 max-h-[90vh] overflow-y-auto rounded-t-(--radius-panel) border border-border bg-surface shadow-(--shadow-panel) sm:inset-x-auto sm:left-1/2 sm:top-1/2 sm:w-full sm:max-w-lg sm:-translate-x-1/2 sm:-translate-y-1/2 sm:rounded-(--radius-panel)"
    role="dialog"
    aria-modal="true"
    aria-labelledby={activeModal === 'short-open' ? 'short-open-title' : activeModal === 'short-cover' ? 'short-cover-title' : activeModal === 'external-transfer-in' ? 'external-transfer-in-title' : activeModal === 'internal-transfer' ? 'internal-transfer-title' : activeModal === 'external-transfer-out' ? 'external-transfer-out-title' : activeModal === 'split' ? 'split-title' : activeModal === 'share-exchange' ? 'share-exchange-title' : activeModal === 'spin-off' ? 'spin-off-title' : activeModal === 'capital-return' ? 'capital-return-title' : undefined}
  >
    <div class="p-6">
      {#if activeModal === 'buy'}
        <BuyForm {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {:else if activeModal === 'sell'}
        <SellForm {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {:else if activeModal === 'short-open'}
        <ShortTradeForm mode="open" {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {:else if activeModal === 'short-cover'}
        <ShortTradeForm mode="cover" {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {:else if activeModal === 'dividend'}
        <DividendForm mode="cash" {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {:else if activeModal === 'reinvested'}
        <DividendForm mode="reinvested" {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {:else if activeModal === 'external-transfer-in'}
        <ExternalTransferInForm {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {:else if activeModal === 'internal-transfer'}
        <InternalTransferForm {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {:else if activeModal === 'external-transfer-out'}
        <ExternalTransferOutForm {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {:else if activeModal === 'split'}
        <SplitForm {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {:else if activeModal === 'share-exchange'}
        <ShareExchangeForm {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {:else if activeModal === 'spin-off'}
        <SpinOffForm {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {:else if activeModal === 'capital-return'}
        <CapitalReturnForm {csrfToken} onSaved={onTradeSaved} onCancel={closeModal} />
      {/if}
    </div>
  </div>
{/if}
