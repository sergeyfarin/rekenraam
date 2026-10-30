<script lang="ts">
  import { page } from '$app/state';
  import { createQuery } from '@tanstack/svelte-query';
  import { m } from '#lib/paraglide/messages.js';
  import { breadcrumbs, routeFor } from '#lib/navigation/routes.ts';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import AccountRegister from '#lib/transactions/account-register.svelte';
  import TransactionWorkspace, { type TransactionWorkspaceCreate } from '#lib/transactions/transaction-workspace.svelte';
  import { accountEntryKind } from '#lib/transactions/entry-account.ts';
  import { investmentInstrumentsQueryOptions } from '#lib/api/investments.ts';
  import { holdingAccountPrefill } from '#lib/investments/investment-prefill.ts';

  const accountID = $derived(Number(page.params.id));

  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const account = $derived(accountsQuery.data?.accounts.find((a) => a.id === accountID));
  // The h1 names the account once it has loaded; until then, the page.
  const accountName = $derived(account?.name || account?.code || m.accounts_title());
  const instrumentsQuery = createQuery(() => ({
    ...investmentInstrumentsQueryOptions(),
    enabled: !!account && accountEntryKind(account) === 'investments'
  }));

  // A new transaction starts in this account (#185). A holding account's
  // activity is entered as investment commands instead, with the holding and
  // its instrument pre-selected (#186).
  const create = $derived.by<TransactionWorkspaceCreate>(() => {
    if (!account) return { kind: 'none' };
    switch (accountEntryKind(account)) {
      case 'editor':
        return { kind: 'editor', defaults: { accountID: account.id } };
      case 'investments':
        return { kind: 'investment', prefill: holdingAccountPrefill(account, instrumentsQuery.data?.instruments ?? []) };
      default:
        return { kind: 'none' };
    }
  });
</script>

<TransactionWorkspace title={accountName} breadcrumbs={breadcrumbs(routeFor(page.url.pathname))} {create}>
  {#snippet list(actions, csrfToken)}
    <AccountRegister {accountID} {csrfToken} {actions} onRowClick={(entry) => actions.showTransaction(entry.transaction_id)} />
  {/snippet}
</TransactionWorkspace>
