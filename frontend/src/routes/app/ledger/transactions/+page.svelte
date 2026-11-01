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

  // The URL seeds the list once, so a report drill-down lands pre-filtered, and
  // is rewritten in place as the bar changes. `replaceState` keeps filter
  // tweaking out of history — Back returns to wherever the link came from.
  const initialFilters = $derived(parseTransactionFilters(page.url.searchParams));
  let filteredAccountID = $state(parseTransactionFilters(page.url.searchParams).accountID);

  function handleFiltersChange(filters: TransactionQueryFilters) {
    filteredAccountID = filters.accountID;
    // Drafts own their lifecycle: keep the view URL clean rather than writing
    // an explicit `status=draft` beside `view=drafts`.
    const urlFilters = view === 'drafts' ? { ...filters, status: undefined } : filters;
    const params = writeTransactionFilters(page.url.searchParams, urlFilters);
    const search = params.toString();
    if (search === page.url.searchParams.toString()) {
      return;
    }
    void goto(`/app/ledger/transactions${search ? `?${search}` : ''}`, {
      replaceState: true,
      reset: false
    });
  }

  // A list filtered to one ordinary account starts a new transaction there (#185).
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const create = $derived.by<TransactionWorkspaceCreate>(() => {
    if (view === 'deleted') return { kind: 'none' };
    const account = (accountsQuery.data?.accounts ?? []).find((item) => item.id === filteredAccountID);
    return { kind: 'editor', defaults: account && isOrdinaryEntryAccount(account) ? { accountID: account.id } : undefined };
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
          {initialFilters}
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
