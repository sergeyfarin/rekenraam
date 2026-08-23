<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { m } from '$lib/paraglide/messages.js';
  import { accountsQueryOptions } from '$lib/api/accounts';
  import { categoriesQueryOptions } from '$lib/api/categories';
  import { currenciesQueryOptions } from '$lib/api/currencies';
  import { investmentInstrumentsQueryOptions } from '$lib/api/investments';
  import { payeesQueryOptions } from '$lib/api/payees';
  import { accountDisplayName } from '$lib/accounts/account-labels';
  import { categoryDisplayName } from '$lib/categories/category-labels';
  import { getLocale } from '$lib/paraglide/runtime.js';
  import ReportFilterSelect from './report-filter-select.svelte';
  import { commodityLabelMap } from './commodity-labels';
  import {
    activeFilterCount,
    clearDimensions,
    selectedIDs,
    withSelectedIDs,
    type FilterOption,
    type ReportFilterDimension,
    type ReportFilterState
  } from './report-filters';

  let {
    filters,
    dimensions,
    onChange
  }: {
    filters: ReportFilterState;
    /** Only the dimensions the active report can express. Net worth has no category or payee. */
    dimensions: ReportFilterDimension[];
    onChange: (filters: ReportFilterState) => void;
  } = $props();

  const locale = $derived(getLocale());
  const collator = $derived(new Intl.Collator(locale, { sensitivity: 'base' }));

  // Archived accounts, categories, and payees stay selectable: a report looks
  // backwards, and a range that predates an archival still has postings on it.
  const accountsQuery = createQuery(() => ({ ...accountsQueryOptions(true, false), enabled: dimensions.includes('account') }));
  const currenciesQuery = createQuery(() => ({ ...currenciesQueryOptions(), enabled: dimensions.includes('commodity') }));
  const instrumentsQuery = createQuery(() => ({
    ...investmentInstrumentsQueryOptions(),
    enabled: dimensions.includes('commodity')
  }));
  const categoriesQuery = createQuery(() => ({
    ...categoriesQueryOptions({ includeArchived: true }),
    enabled: dimensions.includes('category')
  }));
  const payeesQuery = createQuery(() => ({
    ...payeesQueryOptions({ includeArchived: true }),
    enabled: dimensions.includes('payee')
  }));

  function sortOptions(options: FilterOption[]): FilterOption[] {
    return options.sort((a, b) => collator.compare(a.label, b.label));
  }

  // The same rule the endpoints validate: the account dimension means "where the
  // money sat or moved through", so only accounts that hold a balance belong.
  // Income and expense accounts are the category dimension, and offering one
  // here would invite a filter that can only return nothing.
  const accountOptions = $derived(
    sortOptions(
      (accountsQuery.data?.accounts ?? [])
        .filter((account) => account.account_class === 'asset' || account.account_class === 'liability')
        .map((account) => ({ id: account.id, label: accountDisplayName(account) }))
    )
  );

  // Both currencies and the commodities behind held instruments can carry a
  // balance, so a portfolio's securities have to be filterable too — and the
  // report screens read the same map, so anything offered here is also readable
  // in the table, chart caption, print sheet, and CSV.
  const commodityOptions = $derived(
    sortOptions(
      [...commodityLabelMap(currenciesQuery.data?.currencies, instrumentsQuery.data?.instruments)].map(
        ([id, label]) => ({ id, label })
      )
    )
  );

  const categoryOptions = $derived(
    sortOptions(
      (categoriesQuery.data?.categories ?? []).map((category) => ({
        id: category.id,
        label: categoryDisplayName(category)
      }))
    )
  );

  const payeeOptions = $derived(
    sortOptions((payeesQuery.data?.payees ?? []).map((payee) => ({ id: payee.id, label: payee.name })))
  );

  const optionsByDimension = $derived<Record<ReportFilterDimension, FilterOption[]>>({
    account: accountOptions,
    commodity: commodityOptions,
    category: categoryOptions,
    payee: payeeOptions
  });

  const activeCount = $derived(activeFilterCount(filters, dimensions));

  function labelFor(dimension: ReportFilterDimension): string {
    switch (dimension) {
      case 'account':
        return m.reports_filter_accounts();
      case 'commodity':
        return m.reports_filter_commodities();
      case 'category':
        return m.reports_filter_categories();
      case 'payee':
        return m.reports_filter_payees();
    }
  }

  function emptyCopyFor(dimension: ReportFilterDimension): string {
    switch (dimension) {
      case 'account':
        return m.reports_filter_accounts_empty();
      case 'commodity':
        return m.reports_filter_commodities_empty();
      case 'category':
        return m.reports_filter_categories_empty();
      case 'payee':
        return m.reports_filter_payees_empty();
    }
  }

  function isPending(dimension: ReportFilterDimension): boolean {
    switch (dimension) {
      case 'account':
        return accountsQuery.isPending;
      case 'commodity':
        return currenciesQuery.isPending || instrumentsQuery.isPending;
      case 'category':
        return categoriesQuery.isPending;
      case 'payee':
        return payeesQuery.isPending;
    }
  }

  function isError(dimension: ReportFilterDimension): boolean {
    switch (dimension) {
      case 'account':
        return accountsQuery.isError;
      case 'commodity':
        return currenciesQuery.isError || instrumentsQuery.isError;
      case 'category':
        return categoriesQuery.isError;
      case 'payee':
        return payeesQuery.isError;
    }
  }

  function retry(dimension: ReportFilterDimension) {
    switch (dimension) {
      case 'account':
        void accountsQuery.refetch();
        return;
      case 'commodity':
        void currenciesQuery.refetch();
        void instrumentsQuery.refetch();
        return;
      case 'category':
        void categoriesQuery.refetch();
        return;
      case 'payee':
        void payeesQuery.refetch();
    }
  }

  function selectedLabels(dimension: ReportFilterDimension): string[] {
    const ids = new Set(selectedIDs(filters, dimension));
    return optionsByDimension[dimension].filter((option) => ids.has(option.id)).map((option) => option.label);
  }
</script>

<section aria-label={m.reports_filter_legend()} class="space-y-3">
  <div class="flex flex-wrap items-center justify-between gap-2">
    <h3 class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.reports_filter_legend()}</h3>
    {#if activeCount > 0}
      <button
        type="button"
        onclick={() => onChange(clearDimensions(filters, dimensions))}
        class="text-sm font-semibold text-accent underline underline-offset-2 transition hover:opacity-80"
      >
        {m.reports_filter_clear_all()}
      </button>
    {/if}
  </div>

  <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
    {#each dimensions as dimension (dimension)}
      <ReportFilterSelect
        label={labelFor(dimension)}
        options={optionsByDimension[dimension]}
        selected={selectedIDs(filters, dimension)}
        pending={isPending(dimension)}
        error={isError(dimension)}
        onRetry={() => retry(dimension)}
        emptyCopy={emptyCopyFor(dimension)}
        onChange={(ids) => onChange(withSelectedIDs(filters, dimension, ids))}
      />
    {/each}
  </div>

  <!-- Expanding a selection to its descendants only means anything once an
       account is chosen, so the control appears with the selection. -->
  {#if dimensions.includes('account') && filters.accountIDs.length > 0}
    <label class="flex items-center gap-2 text-sm text-foreground">
      <input
        type="checkbox"
        class="size-4 accent-[var(--color-accent)]"
        checked={filters.includeDescendants}
        onchange={(event) =>
          onChange({ ...filters, includeDescendants: event.currentTarget.checked })}
      />
      <span>{m.reports_filter_include_descendants()}</span>
    </label>
  {/if}

  {#if activeCount > 0}
    <!-- A shared link has to explain its own scope: the reader sees what the
         numbers are narrowed to without opening every control. -->
    <dl class="flex flex-wrap gap-x-6 gap-y-1 text-sm">
      {#each dimensions as dimension (dimension)}
        {#if selectedIDs(filters, dimension).length > 0}
          <div class="flex min-w-0 gap-2">
            <dt class="shrink-0 text-muted">{labelFor(dimension)}</dt>
            <dd class="min-w-0 truncate font-medium text-foreground">
              {#each selectedLabels(dimension) as name, index (name)}<!--
                -->{index > 0 ? m.reports_filter_summary_separator() : ''}{name}{/each}
            </dd>
          </div>
        {/if}
      {/each}
    </dl>
  {/if}
</section>
