<script lang="ts">
  import { page } from '$app/state';
  import { createQuery } from '@tanstack/svelte-query';
  import { m } from '#lib/paraglide/messages.js';
  import { breadcrumbs, routeFor } from '#lib/navigation/routes.ts';
  import { accountQueryOptions } from '#lib/api/accounts.ts';
  import { APIClientError } from '#lib/api/client.ts';
  import { ledgerAccountQueryOptions } from '#lib/api/ledger.ts';
  import BrokerageSummary from '#lib/accounts/brokerage-summary.svelte';
  import AccountRegister from '#lib/transactions/account-register.svelte';
  import TransactionWorkspace, { type TransactionWorkspaceCreate } from '#lib/transactions/transaction-workspace.svelte';
  import { accountEntryKind } from '#lib/transactions/entry-account.ts';
  import { investmentInstrumentsQueryOptions } from '#lib/api/investments.ts';
  import { holdingAccountPrefill } from '#lib/investments/investment-prefill.ts';

  const accountID = $derived(Number(page.params.id));

  // The register's own account, read by id: an archived account or one far
  // down a large catalog still names the page and seeds New transaction (#518).
  const accountQuery = createQuery(() => ({ ...ledgerAccountQueryOptions(accountID), enabled: Number.isInteger(accountID) && accountID > 0 }));
  const summary = $derived(accountQuery.data?.accounts[0]);
  // Existing diagnostic links can name income, equity or system accounts,
  // which have registers but are outside the account index's valuation scope.
  const needsReferenceRead = $derived(accountQuery.error instanceof APIClientError && accountQuery.error.status === 404);
  const referenceQuery = createQuery(() => ({ ...accountQueryOptions(accountID), enabled: needsReferenceRead }));
  const account = $derived(summary?.account ?? referenceQuery.data);
  const loadError = $derived(needsReferenceRead ? referenceQuery.isError : accountQuery.isError);
  // The h1 names the account once it has loaded; until then, the page.
  const accountName = $derived(account?.name || account?.code || m.accounts_title());
  const instrumentsQuery = createQuery(() => ({
    ...investmentInstrumentsQueryOptions(),
    enabled: !!account && accountEntryKind(account) === 'investments'
  }));

  // New transaction starts in this account (#185). A holding account starts
  // on the Investment tab with the holding and its instrument pre-selected
  // (#186); its cash tabs ask for a cash account, never posting to the
  // holding (#506).
  const create = $derived.by<TransactionWorkspaceCreate>(() => {
    if (!account) return { kind: 'none' };
    switch (accountEntryKind(account)) {
      case 'editor':
        return { kind: 'entry', defaults: { accountID: account.id } };
      case 'investments':
        return {
          kind: 'entry',
          defaults: { accountID: account.id, tab: 'investment', investment: holdingAccountPrefill(account, instrumentsQuery.data?.instruments ?? []) }
        };
      default:
        return { kind: 'none' };
    }
  });
</script>

<TransactionWorkspace title={accountName} breadcrumbs={breadcrumbs(routeFor(page.url.pathname))} {create}>
  {#snippet list(actions, csrfToken)}
    {#if loadError}<p role="alert" class="mb-4 text-sm text-warning">{m.accounts_error_copy()}</p>{/if}
    {#if summary && accountQuery.data && (account?.account_kind === 'brokerage' || summary.positions.length > 0)}
      <div class="mb-4"><BrokerageSummary {summary} commodities={accountQuery.data.commodities} asOf={accountQuery.data.as_of} /></div>
    {/if}
    <AccountRegister {accountID} {csrfToken} {actions} onRowClick={(entry) => actions.showTransaction(entry.transaction_id)} />
  {/snippet}
</TransactionWorkspace>
