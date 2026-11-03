<script lang="ts">
  import { page } from '$app/state';
  import { createQuery } from '@tanstack/svelte-query';
  import { m } from '#lib/paraglide/messages.js';
  import { breadcrumbs, routeFor } from '#lib/navigation/routes.ts';
  import { categoriesQueryOptions } from '#lib/api/categories.ts';
  import CategoryTransactions from '#lib/transactions/category-transactions.svelte';
  import TransactionWorkspace, { type TransactionWorkspaceCreate } from '#lib/transactions/transaction-workspace.svelte';
  import { categoryDisplayName } from '#lib/categories/category-labels.ts';

  const categoryID = $derived(Number(page.params.id));

  const categoriesQuery = createQuery(() => categoriesQueryOptions({ includeArchived: true }));
  const category = $derived(categoriesQuery.data?.categories.find((c) => c.id === categoryID));
  const categoryName = $derived(category ? categoryDisplayName(category) : m.categories_title());

  // A new transaction starts in this category (#185) when it can take
  // postings; a summary or archived category still allows unassigned entry.
  const create = $derived<TransactionWorkspaceCreate>({
    kind: 'editor',
    defaults: category?.status === 'active' && category.allows_postings ? { categoryID: category.id } : undefined
  });
</script>

<TransactionWorkspace title={categoryName} breadcrumbs={breadcrumbs(routeFor(page.url.pathname))} {create}>
  {#snippet list(actions, csrfToken)}
    <CategoryTransactions {categoryID} {csrfToken} {actions} onRowClick={actions.showDetail} />
  {/snippet}
</TransactionWorkspace>
