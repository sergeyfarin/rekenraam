<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import { spendingQueryOptions } from '$lib/api/ledger';
  import { categoriesQueryOptions } from '$lib/api/categories';
  import { categoryDisplayName } from '$lib/categories/category-labels';
  import { formatQuantity } from '$lib/money/format';
  import { m } from '$lib/paraglide/messages.js';
  import { formatShare, spendingIsMultiCommodity, spendingRows, type SpendingRow } from './spending';
  import type { ReportFilters } from './report-filters';

  let {
    filters,
    locale,
    commodityLabel
  } = $props<{
    filters: ReportFilters;
    locale: string;
    commodityLabel: (commodityID: number) => string;
  }>();

  const query = createQuery(() => ({
    ...spendingQueryOptions({
      startDate: filters.startDate,
      endDate: filters.endDate,
      groupBy: filters.groupBy,
      direction: filters.direction
    })
  }));

  // Category labels are resolved here rather than by the server: built-in
  // categories carry a localized label that only this layer can render. One
  // request per screen, not per row. Archived categories are included because
  // a report over a past period must still be able to name what was spent on.
  const categoriesQuery = createQuery(() => categoriesQueryOptions({ includeArchived: true }));

  const rows = $derived(query.data ? spendingRows(query.data) : []);
  const multiCommodity = $derived(query.data ? spendingIsMultiCommodity(query.data) : false);

  const categoryNameByID = $derived.by(() => {
    const names = new Map<number, string>();
    for (const category of categoriesQuery.data?.categories ?? []) {
      names.set(category.id, categoryDisplayName(category));
    }
    return names;
  });

  function groupLabel(row: SpendingRow): string {
    if (row.group.unassigned) {
      return filters.groupBy === 'payee' ? m.reports_spending_no_payee() : m.reports_spending_no_category();
    }
    if (row.group.payee_id !== undefined) {
      return row.group.payee_label || m.reports_spending_payee_unknown({ payeeId: row.group.payee_id });
    }
    const categoryID = row.group.category_account_id;
    if (categoryID === undefined) {
      return m.reports_spending_no_category();
    }
    return categoryNameByID.get(categoryID) ?? m.reports_spending_category_unknown({ categoryId: categoryID });
  }

  const dimensionHeader = $derived(
    filters.groupBy === 'payee' ? m.reports_column_payee() : m.reports_column_category()
  );
  const amountHeader = $derived(
    filters.direction === 'income' ? m.reports_column_income() : m.reports_column_spending()
  );
  const title = $derived(
    filters.direction === 'income' ? m.reports_income_title() : m.reports_spending_title()
  );
  const copy = $derived(
    filters.direction === 'income' ? m.reports_income_copy() : m.reports_spending_copy()
  );
</script>

{#if query.isPending}
  <StatePanel title={m.reports_spending_loading_title()} copy={m.reports_loading_copy()} />
{:else if query.isError}
  <StatePanel title={m.reports_spending_error_title()} copy={m.reports_error_copy()}>
    <button
      type="button"
      class="rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
      onclick={() => query.refetch()}
    >
      {m.reports_retry()}
    </button>
  </StatePanel>
{:else if rows.length === 0}
  <StatePanel title={m.reports_spending_empty_title()} copy={m.reports_spending_empty_copy()} />
{:else}
  <Panel>
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h2 class="text-lg font-semibold tracking-tight text-foreground">{title}</h2>
        <p class="mt-1 text-sm text-muted">{copy}</p>
      </div>
      <p class="text-sm text-muted">{m.reports_posted_only()}</p>
    </div>

    <!--
      The exclusion policy is stated on the screen, not just in the API
      response: a user reading a spending total needs to know that transfers
      between their own accounts were never in it.
    -->
    <p class="mt-3 text-sm text-muted">{m.reports_spending_transfers_excluded()}</p>

    {#if multiCommodity}
      <p class="mt-4 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground">
        {m.reports_spending_multi_commodity_notice({
          commodity: commodityLabel(query.data?.rank_commodity_id ?? 0)
        })}
      </p>
    {/if}

    <div class="mt-5 overflow-x-auto">
      <table class="w-full min-w-[34rem] border-collapse text-left text-sm">
        <caption class="sr-only">{m.reports_spending_table_caption()}</caption>
        <thead>
          <tr class="border-b border-border text-xs uppercase tracking-[0.12em] text-muted">
            <th scope="col" class="px-3 py-3 font-semibold">{dimensionHeader}</th>
            <th scope="col" class="px-3 py-3 font-semibold">{m.reports_column_commodity()}</th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">{amountHeader}</th>
            <th scope="col" class="px-3 py-3 text-right font-semibold">{m.reports_column_share()}</th>
          </tr>
        </thead>
        <tbody>
          {#each rows as row (row.key)}
            <tr class="border-b border-border/70 last:border-b-0">
              <td class="px-3 py-3 text-foreground">
                {groupLabel(row)}
                {#if row.group.unassigned}
                  <span class="ml-1 text-xs text-muted">{m.reports_spending_unassigned_hint()}</span>
                {/if}
              </td>
              <td class="px-3 py-3 font-medium text-foreground">{commodityLabel(row.commodityId)}</td>
              <td class="px-3 py-3 text-right font-semibold tabular-nums text-foreground">
                {commodityLabel(row.commodityId)}
                {formatQuantity(row.normalQuantityValue, row.quantityScale, locale)}
              </td>
              <td class="px-3 py-3 text-right tabular-nums text-muted">
                {m.reports_share_percent({ share: formatShare(row.shareBasisPoints, locale) })}
              </td>
            </tr>
          {/each}
        </tbody>
        <tfoot>
          {#each query.data?.commodity_totals ?? [] as commodityTotal (commodityTotal.commodity_id)}
            <tr class="border-t-2 border-border font-semibold">
              <td class="px-3 py-3 text-foreground" colspan="2">{m.reports_column_total()}</td>
              <td class="px-3 py-3 text-right tabular-nums text-foreground">
                {commodityLabel(commodityTotal.commodity_id)}
                {formatQuantity(commodityTotal.normal_quantity_value, commodityTotal.quantity_scale, locale)}
              </td>
              <td class="px-3 py-3"></td>
            </tr>
          {/each}
        </tfoot>
      </table>
    </div>
  </Panel>
{/if}
