<script lang="ts">
  import { page } from '$app/state';
  import { createQuery } from '@tanstack/svelte-query';
  import { m } from '#lib/paraglide/messages.js';
  import { breadcrumbs, routeFor } from '#lib/navigation/routes.ts';
  import { accountQueryOptions, accountsQueryKey, getAccount } from '#lib/api/accounts.ts';
  import { ledgerAccountQueryOptions } from '#lib/api/ledger.ts';
  import { findBrokerageAncestor, isHoldingAccount } from '#lib/accounts/brokerage.ts';
  import BrokerageSummary from '#lib/accounts/brokerage-summary.svelte';
  import AccountRegister from '#lib/transactions/account-register.svelte';
  import TransactionWorkspace, { type TransactionWorkspaceCreate } from '#lib/transactions/transaction-workspace.svelte';
  import { accountEntryKind } from '#lib/transactions/entry-account.ts';
  import { investmentInstrumentsQueryOptions } from '#lib/api/investments.ts';
  import { holdingAccountPrefill } from '#lib/investments/investment-prefill.ts';

  const accountID = $derived(Number(page.params.id));

  // The register's own account, read by id: an archived account or one far
  // down a large catalog still names the page and seeds New transaction (#518).
  // Every register, including income, equity and system accounts, needs only
  // this one-account read.
  const accountQuery = createQuery(() => ({ ...accountQueryOptions(accountID), enabled: Number.isInteger(accountID) && accountID > 0 }));
  const account = $derived(accountQuery.data);
  const loadError = $derived(accountQuery.isError);
  const holding = $derived(isHoldingAccount(account));
  // The composed Ledger read (cash, positions, values) is a book-wide
  // valuation, so only a brokerage register, whose summary needs it, asks
  // for it; an ordinary register never pays for it or refetches it on writes.
  const summaryQuery = createQuery(() => ({ ...ledgerAccountQueryOptions(accountID), enabled: account?.account_kind === 'brokerage' }));
  const summary = $derived(summaryQuery.data?.accounts[0]);
  // A holding's register leads back to its brokerage and its instrument; the
  // brokerage is the nearest brokerage ancestor, as Ledger Overview owns it.
  const brokerageQuery = createQuery(() => ({
    queryKey: [...accountsQueryKey, 'brokerage-of', accountID] as const,
    queryFn: () => findBrokerageAncestor(account!, getAccount),
    enabled: holding && !!account?.parent_account_id,
    staleTime: 10_000
  }));
  const pageBreadcrumbs = $derived.by(() => {
    const base = breadcrumbs(routeFor(page.url.pathname));
    const brokerage = brokerageQuery.data;
    return brokerage ? [...base, { label: brokerage.name || brokerage.code || String(brokerage.id), href: `/app/ledger/accounts/${brokerage.id}/register` }] : base;
  });
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

<TransactionWorkspace title={accountName} breadcrumbs={pageBreadcrumbs} {create}>
  {#snippet list(actions, csrfToken)}
    {#if loadError}<p role="alert" class="mb-4 text-sm text-warning">{m.accounts_error_copy()}</p>{/if}
    {#if summary && summaryQuery.data}
      <div class="mb-4"><BrokerageSummary {summary} commodities={summaryQuery.data.commodities} asOf={summaryQuery.data.as_of} /></div>
    {:else if summaryQuery.isError}
      <p role="alert" class="mb-4 text-sm text-warning">{m.accounts_error_copy()}</p>
    {/if}
    {#if holding && account}
      <div class="mb-4 flex flex-wrap gap-x-4">
        {#if account.default_commodity_id}<a class="inline-flex min-h-11 items-center text-sm font-semibold underline" href={`/app/investments/instruments/${account.default_commodity_id}`}>{m.ledger_holding_instrument()}</a>{/if}
        <a class="inline-flex min-h-11 items-center text-sm font-semibold underline" href={`/app/ledger/reconcile?account=${account.id}`}>{m.ledger_holding_reconcile()}</a>
      </div>
    {/if}
    <AccountRegister {accountID} {csrfToken} {actions} onRowClick={(entry) => actions.showTransaction(entry.transaction_id)} />
  {/snippet}
</TransactionWorkspace>
