<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import { m } from '#lib/paraglide/messages.js';
  import TransactionList from '#lib/transactions/transaction-list.svelte';
  import TransactionWorkspace, { type TransactionWorkspaceCreate } from '#lib/transactions/transaction-workspace.svelte';
  import { isOrdinaryEntryAccount } from '#lib/transactions/entry-account.ts';
  import {
    parseTransactionFilters,
    writeTransactionFilters,
    type TransactionQueryFilters
  } from '#lib/transactions/transaction-url-filters.ts';

  const linkedTransactionID = Number(page.url.searchParams.get('transaction_id') ?? 0);

  // The URL seeds the list once, so a report drill-down lands pre-filtered, and
  // is rewritten in place as the bar changes. `replaceState` keeps filter
  // tweaking out of history — Back returns to wherever the link came from.
  const initialFilters = parseTransactionFilters(page.url.searchParams);
  let filteredAccountID = $state(initialFilters.accountID);

  function handleFiltersChange(filters: TransactionQueryFilters) {
    filteredAccountID = filters.accountID;
    const params = writeTransactionFilters(page.url.searchParams, filters);
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
    const account = (accountsQuery.data?.accounts ?? []).find((item) => item.id === filteredAccountID);
    return { kind: 'editor', defaults: account && isOrdinaryEntryAccount(account) ? { accountID: account.id } : undefined };
  });
</script>

<TransactionWorkspace title={m.transactions_page_title()} {create} {linkedTransactionID}>
  {#snippet list(actions, csrfToken)}
    <TransactionList
      {csrfToken}
      {actions}
      {initialFilters}
      onFiltersChange={handleFiltersChange}
      onRowClick={actions.showDetail}
    />
  {/snippet}
</TransactionWorkspace>
