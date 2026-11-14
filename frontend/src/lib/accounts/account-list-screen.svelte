<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { ledgerOverviewQueryOptions } from '#lib/api/ledger.ts';
  import PageHeader from '#lib/components/page-header.svelte';
  import SegmentedControl from '#lib/components/segmented-control.svelte';
  import LedgerInstitutions from './ledger-institutions.svelte';
  import LedgerValue from './ledger-value.svelte';
  import { isHoldingAccount } from './brokerage.ts';
  import { childDependents } from './dependents.ts';
  import { addTransaction, addTransfer } from '#lib/transactions/add-transaction.svelte.ts';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import BuildingComplex from '@lucide/svelte/icons/building-complex';
  import Plus from '@lucide/svelte/icons/plus';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import WalletCards from '@lucide/svelte/icons/wallet-cards';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import Panel from '#lib/components/panel.svelte';
  import StatePanel from '#lib/components/state-panel.svelte';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import { currentBookQueryOptions } from '#lib/api/books.ts';
  import { forecastQueryKey } from '#lib/api/forecast.ts';
  import {
    accountsQueryKey,
    archiveAccount,
    closeAccount,
    deleteAccount,
    type AccountResponse
  } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, currencyCatalogQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    deleteInstitution,
    archiveInstitution,
    restoreInstitution,
    type InstitutionResponse
  } from '#lib/api/institutions.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { localizedCurrencyCatalog } from '#lib/install-gate/currency-options.ts';
  import AccountEditor from './account-editor.svelte';
  import AccountFilterBar from './account-filter-bar.svelte';
  import AccountSummaryStats from './account-summary-stats.svelte';
  import AccountTreeSection from './account-tree-section.svelte';
  import InstitutionEditor from './institution-editor.svelte';
  import {
    accountCountryLabel,
    accountDisplayName,
    accountKindGroupLabel,
    accountInstitutionLabel,
    accountKindLabel,
    institutionCountryLabel,
    institutionKindLabel,
    isCategoryAccount
  } from './account-labels';
  import type { AccountTypeFilter, StatusFilter } from './account-list-options';

  type EditorState =
    | { type: 'none' }
    | { type: 'account-create' }
    | { type: 'account-edit'; account: AccountResponse }
    | { type: 'institution-create' }
    | { type: 'institution-edit'; institution: InstitutionResponse };

  let editor = $state<EditorState>({ type: 'none' });

  const overview = createQuery(() => ledgerOverviewQueryOptions());
  const accountsQuery = $derived({ data: overview.data ? { accounts: overview.data.accounts.map(row => row.account) } : undefined, error: overview.error, isError: overview.isError, isPending: overview.isPending, refetch: () => overview.refetch() });
  const institutionsQuery = $derived({ data: overview.data ? { institutions: overview.data.institutions } : undefined, error: overview.error, isError: overview.isError, isPending: overview.isPending, refetch: () => overview.refetch() });
  const currenciesQuery = createQuery(() => ({ ...currenciesQueryOptions(), enabled: editor.type === 'account-create' || editor.type === 'account-edit' }));
  const currencyCatalogQuery = createQuery(() => ({ ...currencyCatalogQueryOptions(), enabled: editor.type === 'account-create' || editor.type === 'account-edit' }));
  const currentBookQuery = createQuery(() => ({ ...currentBookQueryOptions(), enabled: editor.type === 'account-create' || editor.type === 'account-edit' }));
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const queryClient = useQueryClient();

  let statusFilter = $state<StatusFilter>('active');
  let accountTypeFilter = $state<AccountTypeFilter>('all');
  let query = $state('');
  let actionError = $state<unknown>(undefined);
  let actionPendingKey = $state('');
  let collapsedNodeIDs = $state<Set<string>>(new Set());

  const view = $derived(page.url.searchParams.get('view') === 'institutions' ? 'institutions' : 'accounts');
  function changeView(value: string) { const url = new URL(page.url.href); url.searchParams.set('view', value); void goto(url, { reset: false }); }
  async function institutionAction(institution: InstitutionResponse, restore = false) {
    await runAccountAction(`institution:${institution.id}`, async token => { if (restore) await restoreInstitution(institution.id, token); else await archiveInstitution(institution.id, token); });
  }
  const countryNames = $derived(new Intl.DisplayNames(getLocale(), { type: 'region' }));
  const currencyDisplayNames = $derived(new Intl.DisplayNames(getLocale(), { type: 'currency' }));

  const institutionsByID = $derived.by(() => {
    return new Map<number, InstitutionResponse>(
      institutionsQuery.data?.institutions.map((institution) => [institution.id, institution]) ?? []
    );
  });

  const currenciesByID = $derived.by(() => {
    return new Map<number, CurrencyResponse>(
      currenciesQuery.data?.currencies.map((currency) => [currency.id, currency]) ?? []
    );
  });
  const currencyCatalog = $derived(localizedCurrencyCatalog(currencyCatalogQuery.data?.currencies ?? [], currencyDisplayNames));

  const allUserAccounts = $derived.by(() => {
    return accountsQuery.data?.accounts.filter((account) => !account.is_system) ?? [];
  });

  const userAccounts = $derived.by(() => {
    return allUserAccounts.filter((account) => !isCategoryAccount(account) && account.account_class !== 'equity' && !isHoldingAccount(account));
  });

  // Archived accounts stay out of every status filter, as before #448: their
  // institutions may be archived too, and the tree has no root for them.
  const manageableAccounts = $derived.by(() => {
    return userAccounts.filter((account) => account.status !== 'archived');
  });

  // A brokerage row folds in its holdings, cash and other children, so they
  // are never tree rows. They are still children of their parent and members
  // of their institution: the server refuses to archive or delete a parent or
  // institution they depend on, so the actions must count them (#567 review).
  const foldedAccounts = $derived(overview.data?.accounts.flatMap((row) => [...row.holding_accounts, ...row.cash_accounts, ...row.other_accounts]) ?? []);
  // A holding with no parent account is its own Ledger row; it is shown
  // apart from the institution tree so its value is never missing.
  const allUnparentedHoldings = $derived(
    overview.data?.accounts.filter((row) => isHoldingAccount(row.account) && row.account.status !== 'archived') ?? []
  );
  // The panel follows the same search, status and type filters as the tree.
  const unparentedHoldings = $derived.by(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();
    return allUnparentedHoldings.filter((row) => accountMatchesFilters(row.account, normalizedQuery));
  });

  const activeInstitutions = $derived.by(() => {
    return institutionsQuery.data?.institutions.filter((institution) => institution.status === 'active') ?? [];
  });

  const institutionAccountCounts = $derived.by(() => {
    const counts = new Map<number, number>();

    for (const account of [...allUserAccounts, ...foldedAccounts]) {
      if (account.institution_id) {
        counts.set(account.institution_id, (counts.get(account.institution_id) ?? 0) + 1);
      }
    }

    return counts;
  });

  // Every child of a parent, including the accounts folded into brokerage
  // rows and the holdings listed apart (they are in allUserAccounts).
  const childDependentCounts = $derived(childDependents([...allUserAccounts, ...foldedAccounts]));

  const visibleAccounts = $derived.by(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();
    const matchedAccounts = manageableAccounts.filter((account) => accountMatchesFilters(account, normalizedQuery));
    const visibleByID = new Map(matchedAccounts.map((account) => [account.id, account]));

    if (normalizedQuery !== '') {
      addVisibleAccountAncestors(visibleByID);
    }

    return Array.from(visibleByID.values());
  });

  const visibleInstitutions = $derived.by(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();

    return activeInstitutions.filter((institution) => {
      const hasVisibleAccounts = visibleAccounts.some((account) => account.institution_id === institution.id);
      if (hasVisibleAccounts) {
        return true;
      }

      if (normalizedQuery !== '') {
        return institutionMatchesQuery(institution, normalizedQuery);
      }

      return accountTypeFilter === 'all' && statusFilter !== 'closed';
    });
  });

  const visibleCount = $derived(visibleAccounts.length);
  const activeCount = $derived(userAccounts.filter((account) => account.status === 'active').length);
  const closedCount = $derived(userAccounts.filter((account) => account.status === 'closed').length);
  const shellError = $derived(
    accountsQuery.error ??
      institutionsQuery.error ??
      currenciesQuery.error ??
      currencyCatalogQuery.error ??
      currentBookQuery.error ??
      sessionQuery.error
  );
  const csrfToken = $derived(sessionQuery.data?.csrf_token);

  const screenState = $derived.by<'loading' | 'error' | 'empty' | 'ready'>(() => {
    if (
      accountsQuery.isPending ||
      institutionsQuery.isPending ||
      sessionQuery.isPending
    ) {
      return 'loading';
    }

    if (
      accountsQuery.isError ||
      institutionsQuery.isError ||
      sessionQuery.isError
    ) {
      return 'error';
    }

    // A book whose only accounts are unparented holdings still shows them.
    if (manageableAccounts.length === 0 && allUnparentedHoldings.length === 0 && (institutionsQuery.data?.institutions.length ?? 0) === 0) {
      return 'empty';
    }

    return 'ready';
  });

  function accountMatchesFilters(account: AccountResponse, normalizedQuery: string): boolean {
    if (statusFilter !== 'all' && account.status !== statusFilter) {
      return false;
    }

    if (accountTypeFilter !== 'all' && account.account_kind !== accountTypeFilter) {
      return false;
    }

    if (normalizedQuery === '') {
      return true;
    }

    const haystack = [
      accountDisplayName(account),
      account.code ?? '',
      accountKindLabel(account.account_kind),
      accountKindGroupLabel(account.account_kind),
      accountInstitutionLabel(account, institutionsByID),
      accountCountryLabel(account, countryNames),
      account.number_last4 ?? ''
    ]
      .join(' ')
      .toLocaleLowerCase();

    return haystack.includes(normalizedQuery);
  }

  function addVisibleAccountAncestors(visibleByID: Map<number, AccountResponse>) {
    const accountsByID = new Map(manageableAccounts.map((account) => [account.id, account]));

    for (const account of Array.from(visibleByID.values())) {
      let parentID = account.parent_account_id;
      const seen = new Set<number>([account.id]);

      while (parentID) {
        if (seen.has(parentID)) {
          break;
        }
        seen.add(parentID);

        const parent = accountsByID.get(parentID);
        if (!parent || parent.institution_id !== account.institution_id) {
          break;
        }
        if (statusFilter !== 'all' && parent.status !== statusFilter) {
          break;
        }
        if (accountTypeFilter !== 'all' && parent.account_kind !== accountTypeFilter) {
          break;
        }

        visibleByID.set(parent.id, parent);
        parentID = parent.parent_account_id;
      }
    }
  }

  function institutionMatchesQuery(institution: InstitutionResponse, normalizedQuery: string): boolean {
    const haystack = [
      institution.name,
      institutionKindLabel(institution.kind),
      institutionCountryLabel(institution, countryNames),
      institution.website ?? ''
    ]
      .join(' ')
      .toLocaleLowerCase();

    return haystack.includes(normalizedQuery);
  }

  function toggleNode(nodeID: string) {
    const next = new Set(collapsedNodeIDs);
    if (next.has(nodeID)) {
      next.delete(nodeID);
    } else {
      next.add(nodeID);
    }
    collapsedNodeIDs = next;
  }

  async function refreshAccounts() {
    await Promise.all([
      accountsQuery.refetch(),
      queryClient.invalidateQueries({ queryKey: accountsQueryKey }),
      currenciesQuery.refetch(),
      currencyCatalogQuery.refetch(),
      currentBookQuery.refetch(),
      sessionQuery.refetch(),
      queryClient.invalidateQueries({ queryKey: forecastQueryKey })
    ]);
  }

  async function getCSRFToken(): Promise<string> {
    if (csrfToken) {
      return csrfToken;
    }

    const refreshedSession = await sessionQuery.refetch();
    const token = refreshedSession.data?.csrf_token;
    if (!token) {
      throw new Error(m.accounts_form_missing_session());
    }

    return token;
  }

  async function handleSaved() {
    editor = { type: 'none' };
    await refreshAccounts();
  }

  async function handleInstitutionSaved(institution: InstitutionResponse) {
    await refreshAccounts();
    editor = { type: 'none' };

    if (manageableAccounts.length === 0) {
      editor = { type: 'account-create' };
    }
  }

  async function handleCloseAccount(account: AccountResponse) {
    await runAccountAction(`close:${account.id}`, async (token) => {
      await closeAccount(account.id, token);
    });
  }

  async function handleArchiveAccount(account: AccountResponse) {
    await runAccountAction(`archive:${account.id}`, async (token) => {
      await archiveAccount(account.id, token);
    });
  }

  async function handleDeleteAccount(account: AccountResponse) {
    if (!confirm(m.accounts_delete_confirm({ name: accountDisplayName(account) }))) {
      return;
    }

    await runAccountAction(`delete:${account.id}`, async (token) => {
      await deleteAccount(account.id, token);
    });
  }

  async function handleDeleteInstitution(institution: InstitutionResponse) {
    if (!confirm(m.institutions_delete_confirm({ name: institution.name }))) {
      return;
    }

    await runAccountAction(`institution-delete:${institution.id}`, async (token) => {
      await deleteInstitution(institution.id, token);
    });
  }

  async function runAccountAction(key: string, action: (csrfToken: string) => Promise<void>) {
    actionPendingKey = key;
    actionError = undefined;

    try {
      const token = await getCSRFToken();
      await action(token);
      await refreshAccounts();
    } catch (error) {
      actionError = error;
    } finally {
      actionPendingKey = '';
    }
  }
</script>

<PageHeader title={m.nav_ledger()}>
  {#snippet actions()}
    <button type="button" class="min-h-11 rounded-(--radius-control) bg-accent px-4 text-sm font-semibold text-accent-foreground" onclick={() => addTransaction()}>{m.transactions_form_create_title()}</button>
    <button type="button" class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 text-sm" onclick={() => addTransfer()}>{m.home_shortcut_transfer()}</button>
    <a class="inline-flex min-h-11 items-center rounded-(--radius-control) border border-border bg-control px-3 text-sm" href="/app/ledger/import">{m.import_title()}</a>
    <a class="inline-flex min-h-11 items-center rounded-(--radius-control) border border-border bg-control px-3 text-sm" href="/app/ledger/reconcile">{m.reconcile_title()}</a>
  {/snippet}
</PageHeader>
<div class="mb-4 overflow-x-auto pb-1"><SegmentedControl legend={m.nav_ledger()} label="" value={view} onChange={changeView} options={[{ value: 'accounts', label: m.accounts_title() }, { value: 'institutions', label: m.ledger_institutions() }]} /></div>
<div class="space-y-4">
  <Panel variant="toolbar">
    <div class="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
      {#if view === 'accounts'}
      <AccountSummaryStats {activeCount} {closedCount} {visibleCount} />
      <div class="min-w-0 flex-1">
        <AccountFilterBar bind:query bind:statusFilter bind:accountTypeFilter />
      </div>
      {/if}
      <div class="flex flex-wrap gap-2 xl:justify-end">
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-95"
          onclick={() => (editor = { type: 'account-create' })}
        >
          <Plus size={16} aria-hidden="true" />
          {m.accounts_add_account()}
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          onclick={() => (editor = { type: 'institution-create' })}
        >
          <BuildingComplex size={16} aria-hidden="true" />
          {m.institutions_add_institution()}
        </button>
      </div>
    </div>
  </Panel>

  <APIFormError error={actionError} id="accounts-action-error" />

  {#if editor.type !== 'none'}
    <Panel>
      {#if (editor.type === 'account-create' || editor.type === 'account-edit') && (currenciesQuery.isPending || currencyCatalogQuery.isPending || currentBookQuery.isPending)}
        <p role="status" class="text-sm text-muted">{m.accounts_loading_copy()}</p>
      {:else if (editor.type === 'account-create' || editor.type === 'account-edit') && (currenciesQuery.isError || currencyCatalogQuery.isError || currentBookQuery.isError)}
        <APIFormError error={currenciesQuery.error ?? currencyCatalogQuery.error ?? currentBookQuery.error} />
        <button type="button" class="min-h-11 text-sm underline" onclick={refreshAccounts}>{m.accounts_retry()}</button>
      {:else if editor.type === 'account-create'}
        <AccountEditor
          mode="create"
          accounts={manageableAccounts}
          institutions={institutionsQuery.data?.institutions ?? []}
          currencies={currenciesQuery.data?.currencies ?? []}
          {currencyCatalog}
          defaultCurrencyCommodityID={currentBookQuery.data?.default_currency_commodity_id ?? undefined}
          {csrfToken}
          onSaved={handleSaved}
          onCancel={() => (editor = { type: 'none' })}
          onQuickInstitution={() => (editor = { type: 'institution-create' })}
        />
      {:else if editor.type === 'account-edit'}
        <AccountEditor
          mode="edit"
          account={editor.account}
          accounts={manageableAccounts}
          institutions={institutionsQuery.data?.institutions ?? []}
          currencies={currenciesQuery.data?.currencies ?? []}
          {currencyCatalog}
          defaultCurrencyCommodityID={currentBookQuery.data?.default_currency_commodity_id ?? undefined}
          {csrfToken}
          onSaved={handleSaved}
          onCancel={() => (editor = { type: 'none' })}
          onQuickInstitution={() => (editor = { type: 'institution-create' })}
        />
      {:else if editor.type === 'institution-create'}
        <InstitutionEditor
          mode="create"
          {csrfToken}
          onSaved={handleInstitutionSaved}
          onCancel={() => (editor = { type: 'none' })}
        />
      {:else if editor.type === 'institution-edit'}
        <InstitutionEditor
          mode="edit"
          institution={editor.institution}
          {csrfToken}
          onSaved={handleInstitutionSaved}
          onCancel={() => (editor = { type: 'none' })}
        />
      {/if}
    </Panel>
  {/if}

  {#if screenState === 'loading'}
    <StatePanel title={m.accounts_loading_title()} copy={m.accounts_loading_copy()} />
  {:else if screenState === 'error'}
    <StatePanel title={m.accounts_error_title()} copy={m.accounts_error_copy()}>
      <APIFormError error={shellError} id="accounts-error" />
      <button
        type="button"
        class="inline-flex items-center gap-2 rounded-(--radius-control) bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-95"
        onclick={refreshAccounts}
      >
        <RefreshCw size={16} aria-hidden="true" />
        {m.accounts_retry()}
      </button>
    </StatePanel>
  {:else if screenState === 'empty' && view !== 'institutions'}
    <StatePanel title={m.accounts_empty_title()} copy={m.accounts_empty_copy()}>
      <div class="flex flex-wrap justify-center gap-2">
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-95"
          onclick={() => (editor = { type: 'account-create' })}
        >
          <WalletCards size={16} aria-hidden="true" />
          {m.accounts_add_account()}
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          onclick={() => (editor = { type: 'institution-create' })}
        >
          <BuildingComplex size={16} aria-hidden="true" />
          {m.institutions_add_institution()}
        </button>
      </div>
    </StatePanel>
  {:else if view === 'institutions'}
    <LedgerInstitutions institutions={institutionsQuery.data?.institutions ?? []} accounts={overview.data?.accounts ?? []} onEdit={(institution) => (editor = { type: 'institution-edit', institution })} onArchive={(institution) => institutionAction(institution)} onRestore={(institution) => institutionAction(institution, true)} pending={actionPendingKey} />
  {:else if visibleAccounts.length === 0 && visibleInstitutions.length === 0 && unparentedHoldings.length > 0}
    <!-- Only unparented holdings match: they are listed below. -->
  {:else if visibleAccounts.length === 0 && visibleInstitutions.length === 0}
    <StatePanel title={m.accounts_no_results_title()} copy={m.accounts_no_results_copy()} />
  {:else}
    <AccountTreeSection
      accounts={visibleAccounts}
      values={new Map(overview.data?.accounts.map(row => [row.account.id, row.value]) ?? [])}
      commodities={overview.data?.commodities ?? []}
      institutions={visibleInstitutions}
      {currenciesByID}
      {countryNames}
      {collapsedNodeIDs}
      onToggle={toggleNode}
      onEditAccount={(account) => (editor = { type: 'account-edit', account })}
      onCloseAccount={handleCloseAccount}
      onArchiveAccount={handleArchiveAccount}
      onDeleteAccount={handleDeleteAccount}
      onEditInstitution={(institution) => (editor = { type: 'institution-edit', institution })}
      onDeleteInstitution={handleDeleteInstitution}
      {institutionAccountCounts}
      {childDependentCounts}
      {actionPendingKey}
    />
  {/if}
  {#if view === 'accounts' && screenState === 'ready' && unparentedHoldings.length > 0}
    <Panel>
      <h2 class="font-semibold">{m.ledger_unparented_holdings()}</h2>
      <p class="mt-1 text-sm text-muted">{m.ledger_unparented_holdings_copy()}</p>
      <ul class="mt-3 divide-y divide-border">
        {#each unparentedHoldings as row (row.account.id)}
          <li class="flex min-w-0 flex-wrap items-center justify-between gap-3 py-2 text-sm">
            <a class="inline-flex min-h-11 items-center wrap-anywhere font-semibold underline" href={`/app/ledger/accounts/${row.account.id}/register`}>{accountDisplayName(row.account)}</a>
            <LedgerValue value={row.value} commodities={overview.data?.commodities ?? []} />
          </li>
        {/each}
      </ul>
    </Panel>
  {/if}
</div>
