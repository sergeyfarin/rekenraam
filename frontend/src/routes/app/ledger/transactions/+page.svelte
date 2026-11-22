<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import { m } from '#lib/paraglide/messages.js';
  import TransactionList from '#lib/transactions/transaction-list.svelte';
  import TransactionWorkspace, { type TransactionWorkspaceCreate } from '#lib/transactions/transaction-workspace.svelte';
  import DeletedTransactions from '#lib/transactions/deleted-transactions.svelte';
  import TransactionViewTabs from '#lib/transactions/transaction-view-tabs.svelte';
  import { parseTransactionView } from '#lib/transactions/transaction-view.ts';
  import { isOrdinaryEntryAccount } from '#lib/transactions/entry-account.ts';
  import {
    parseTransactionFilters,
    writeTransactionFilters,
    type TransactionQueryFilters
  } from '#lib/transactions/transaction-url-filters.ts';

  const linkedTransactionID = $derived(Number(page.url.searchParams.get('transaction_id') ?? 0));
  const view = $derived(parseTransactionView(page.url.searchParams));

  // Drill-down links can arrive while this page is already mounted.
  const filters = $derived(parseTransactionFilters(page.url.searchParams));

  function handleFiltersChange(next: TransactionQueryFilters) {
    // Drafts own their lifecycle: keep the view URL clean rather than writing
    // an explicit `status=draft` beside `view=drafts`.
    const urlFilters = view === 'drafts' ? { ...next, status: undefined } : next;
    const url = new URL(page.url.href);
    url.search = writeTransactionFilters(url.searchParams, urlFilters).toString();
    if (url.search !== page.url.search) void goto(url, { replace: true, reset: false });
  }

  // A list filtered to one ordinary account starts a new transaction there (#185).
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const create = $derived.by<TransactionWorkspaceCreate>(() => {
    if (view === 'deleted') return { kind: 'none' };
    const account = (accountsQuery.data?.accounts ?? []).find((item) => item.id === filters.accountID);
    return { kind: 'entry', defaults: account && isOrdinaryEntryAccount(account) ? { accountID: account.id } : undefined };
  });
</script>

<TransactionWorkspace
  title={m.transactions_page_title()}
  {create}
  linkedTransactionID={view === 'deleted' ? 0 : linkedTransactionID}
>
  {#snippet header()}
    <TransactionViewTabs {view} params={page.url.searchParams} />
  {/snippet}
  {#snippet list(actions, csrfToken)}
    {#if view === 'deleted'}
      <DeletedTransactions />
    {:else}
      {#key view}
        <TransactionList
          {csrfToken}
          {actions}
          {filters}
          onFiltersChange={handleFiltersChange}
          onRowClick={actions.showDetail}
          lockStatus={view === 'drafts' ? 'draft' : undefined}
          emptyTitle={view === 'drafts' ? m.transactions_drafts_empty_title() : undefined}
          emptyCopy={view === 'drafts' ? m.transactions_drafts_empty_copy() : undefined}
        />
      {/key}
    {/if}
  {/snippet}
</TransactionWorkspace>
