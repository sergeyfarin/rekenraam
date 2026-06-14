<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Building2 from '@lucide/svelte/icons/building-2';
  import Plus from '@lucide/svelte/icons/plus';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import WalletCards from '@lucide/svelte/icons/wallet-cards';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import { authSessionQueryOptions } from '$lib/api/auth';
  import {
    accountsQueryOptions,
    archiveAccount,
    closeAccount,
    deleteAccount,
    type AccountResponse
  } from '$lib/api/accounts';
  import { currenciesQueryOptions, type CurrencyResponse } from '$lib/api/currencies';
  import {
    deleteInstitution,
    institutionsQueryOptions,
    type InstitutionResponse
  } from '$lib/api/institutions';
  import { m } from '$lib/paraglide/messages.js';
  import AccountEditor from './account-editor.svelte';
  import AccountFilterBar from './account-filter-bar.svelte';
  import AccountSummaryStats from './account-summary-stats.svelte';
  import AccountTreeSection from './account-tree-section.svelte';
  import InstitutionEditor from './institution-editor.svelte';
  import {
    accountClassLabel,
    accountCountryLabel,
    accountDisplayName,
    accountInstitutionLabel,
    accountKindLabel,
    institutionCountryLabel,
    institutionKindLabel
  } from './account-labels';
  import type { ClassFilter, StatusFilter } from './account-list-options';

  type EditorState =
    | { type: 'none' }
    | { type: 'account-create' }
    | { type: 'account-edit'; account: AccountResponse }
    | { type: 'institution-create' }
    | { type: 'institution-edit'; institution: InstitutionResponse };

  const accountsQuery = createQuery(() => accountsQueryOptions(true, false));
  const institutionsQuery = createQuery(() => institutionsQueryOptions(false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());
  const sessionQuery = createQuery(() => authSessionQueryOptions());

  let statusFilter = $state<StatusFilter>('all');
  let classFilter = $state<ClassFilter>('all');
  let query = $state('');
  let editor = $state<EditorState>({ type: 'none' });
  let actionError = $state<unknown>(undefined);
  let actionPendingKey = $state('');
  let collapsedNodeIDs = $state<Set<string>>(new Set());

  const countryNames = new Intl.DisplayNames(undefined, { type: 'region' });

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

  const userAccounts = $derived.by(() => {
    return accountsQuery.data?.accounts.filter((account) => !account.is_system) ?? [];
  });

  const manageableAccounts = $derived.by(() => {
    return userAccounts.filter((account) => account.status !== 'archived');
  });

  const activeInstitutions = $derived.by(() => {
    return institutionsQuery.data?.institutions.filter((institution) => institution.status === 'active') ?? [];
  });

  const institutionAccountCounts = $derived.by(() => {
    const counts = new Map<number, number>();

    for (const account of userAccounts) {
      if (account.institution_id) {
        counts.set(account.institution_id, (counts.get(account.institution_id) ?? 0) + 1);
      }
    }

    return counts;
  });

  const allAccountChildCounts = $derived.by(() => {
    const counts = new Map<number, number>();

    for (const account of userAccounts) {
      if (account.parent_account_id) {
        counts.set(account.parent_account_id, (counts.get(account.parent_account_id) ?? 0) + 1);
      }
    }

    return counts;
  });

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

      return classFilter === 'all' && statusFilter !== 'closed';
    });
  });

  const visibleCount = $derived(visibleAccounts.length);
  const activeCount = $derived(userAccounts.filter((account) => account.status === 'active').length);
  const closedCount = $derived(userAccounts.filter((account) => account.status === 'closed').length);
  const shellError = $derived(accountsQuery.error ?? institutionsQuery.error ?? currenciesQuery.error ?? sessionQuery.error);
  const csrfToken = $derived(sessionQuery.data?.csrf_token);

  const screenState = $derived.by<'loading' | 'error' | 'empty' | 'ready'>(() => {
    if (accountsQuery.isPending || institutionsQuery.isPending || currenciesQuery.isPending || sessionQuery.isPending) {
      return 'loading';
    }

    if (accountsQuery.isError || institutionsQuery.isError || currenciesQuery.isError || sessionQuery.isError) {
      return 'error';
    }

    if (manageableAccounts.length === 0 && activeInstitutions.length === 0) {
      return 'empty';
    }

    return 'ready';
  });

  function accountMatchesFilters(account: AccountResponse, normalizedQuery: string): boolean {
    if (statusFilter !== 'all' && account.status !== statusFilter) {
      return false;
    }

    if (classFilter !== 'all' && account.account_class !== classFilter) {
      return false;
    }

    if (normalizedQuery === '') {
      return true;
    }

    const haystack = [
      accountDisplayName(account),
      account.code ?? '',
      accountKindLabel(account.account_kind),
      accountClassLabel(account.account_class),
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
        if (classFilter !== 'all' && parent.account_class !== classFilter) {
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
      institutionsQuery.refetch(),
      currenciesQuery.refetch(),
      sessionQuery.refetch()
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

<div class="space-y-4">
  <Panel variant="toolbar">
    <div class="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
      <AccountSummaryStats {activeCount} {closedCount} {visibleCount} />
      <div class="min-w-0 flex-1">
        <AccountFilterBar bind:query bind:statusFilter bind:classFilter />
      </div>
      <div class="flex flex-wrap gap-2 xl:justify-end">
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
          onclick={() => (editor = { type: 'account-create' })}
        >
          <Plus size={16} aria-hidden="true" />
          {m.accounts_add_account()}
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          onclick={() => (editor = { type: 'institution-create' })}
        >
          <Building2 size={16} aria-hidden="true" />
          {m.institutions_add_institution()}
        </button>
      </div>
    </div>
  </Panel>

  <APIFormError error={actionError} id="accounts-action-error" />

  {#if editor.type !== 'none'}
    <Panel>
      {#if editor.type === 'account-create'}
        <AccountEditor
          mode="create"
          accounts={manageableAccounts}
          institutions={activeInstitutions}
          currencies={currenciesQuery.data?.currencies ?? []}
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
          institutions={activeInstitutions}
          currencies={currenciesQuery.data?.currencies ?? []}
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
        class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
        onclick={refreshAccounts}
      >
        <RefreshCw size={16} aria-hidden="true" />
        {m.accounts_retry()}
      </button>
    </StatePanel>
  {:else if screenState === 'empty'}
    <StatePanel title={m.accounts_empty_title()} copy={m.accounts_empty_copy()}>
      <div class="flex flex-wrap justify-center gap-2">
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
          onclick={() => (editor = { type: 'account-create' })}
        >
          <WalletCards size={16} aria-hidden="true" />
          {m.accounts_add_account()}
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-[var(--radius-control)] border border-border bg-control px-4 py-2.5 text-sm font-semibold text-foreground transition hover:bg-control-hover"
          onclick={() => (editor = { type: 'institution-create' })}
        >
          <Building2 size={16} aria-hidden="true" />
          {m.institutions_add_institution()}
        </button>
      </div>
    </StatePanel>
  {:else if visibleAccounts.length === 0 && visibleInstitutions.length === 0}
    <StatePanel title={m.accounts_no_results_title()} copy={m.accounts_no_results_copy()} />
  {:else}
    <AccountTreeSection
      accounts={visibleAccounts}
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
      {allAccountChildCounts}
      {actionPendingKey}
    />
  {/if}
</div>
