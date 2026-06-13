<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Building2 from '@lucide/svelte/icons/building-2';
  import Edit3 from '@lucide/svelte/icons/edit-3';
  import Plus from '@lucide/svelte/icons/plus';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import Trash2 from '@lucide/svelte/icons/trash-2';
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
  import AccountGroupSection from './account-group-section.svelte';
  import AccountSummaryStats from './account-summary-stats.svelte';
  import InstitutionEditor from './institution-editor.svelte';
  import {
    accountClassLabel,
    accountClassRank,
    accountCountryLabel,
    accountDisplayName,
    accountInstitutionLabel,
    accountKindLabel,
    accountStatusLabel
  } from './account-labels';
  import type { ClassFilter, GroupMode, SortMode, StatusFilter } from './account-list-options';

  type AccountGroup = {
    key: string;
    label: string;
    accounts: AccountResponse[];
    rank: number;
  };

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

  let groupMode = $state<GroupMode>('institution');
  let sortMode = $state<SortMode>('institution');
  let statusFilter = $state<StatusFilter>('all');
  let classFilter = $state<ClassFilter>('all');
  let query = $state('');
  let editor = $state<EditorState>({ type: 'none' });
  let actionError = $state<unknown>(undefined);
  let actionPendingKey = $state('');

  const countryNames = new Intl.DisplayNames(undefined, { type: 'region' });
  const collator = new Intl.Collator(undefined, { sensitivity: 'base', numeric: true });

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

  const visibleAccounts = $derived.by(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();

    return manageableAccounts
      .filter((account) => {
        if (statusFilter !== 'all' && account.status !== statusFilter) {
          return false;
        }

        if (statusFilter === 'all' && account.status === 'archived') {
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
      })
      .toSorted(compareAccounts);
  });

  const groupedAccounts = $derived.by(() => {
    const groups = new Map<string, AccountGroup>();

    for (const account of visibleAccounts) {
      const descriptor = groupDescriptor(account);
      const group = groups.get(descriptor.key);

      if (group) {
        group.accounts.push(account);
      } else {
        groups.set(descriptor.key, {
          ...descriptor,
          accounts: [account]
        });
      }
    }

    return Array.from(groups.values()).toSorted((left, right) => {
      if (left.rank !== right.rank) {
        return left.rank - right.rank;
      }

      return collator.compare(left.label, right.label);
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

    if (manageableAccounts.length === 0) {
      return 'empty';
    }

    return 'ready';
  });

  function compareAccounts(left: AccountResponse, right: AccountResponse): number {
    switch (sortMode) {
      case 'class': {
        const classRank = accountClassRank(left.account_class) - accountClassRank(right.account_class);
        return classRank || compareByName(left, right);
      }
      case 'institution': {
        const institutionComparison = collator.compare(
          accountInstitutionLabel(left, institutionsByID),
          accountInstitutionLabel(right, institutionsByID)
        );
        return institutionComparison || compareByName(left, right);
      }
      case 'country': {
        const countryComparison = collator.compare(
          accountCountryLabel(left, countryNames),
          accountCountryLabel(right, countryNames)
        );
        return countryComparison || compareByName(left, right);
      }
      case 'kind': {
        const kindComparison = collator.compare(accountKindLabel(left.account_kind), accountKindLabel(right.account_kind));
        return kindComparison || compareByName(left, right);
      }
      case 'updated': {
        const updatedComparison = Date.parse(right.updated_at) - Date.parse(left.updated_at);
        return updatedComparison || compareByName(left, right);
      }
      default:
        return compareByName(left, right);
    }
  }

  function compareByName(left: AccountResponse, right: AccountResponse): number {
    return collator.compare(accountDisplayName(left), accountDisplayName(right)) || left.id - right.id;
  }

  function groupDescriptor(account: AccountResponse): Omit<AccountGroup, 'accounts'> {
    switch (groupMode) {
      case 'class':
        return {
          key: `class:${account.account_class}`,
          label: accountClassLabel(account.account_class),
          rank: accountClassRank(account.account_class)
        };
      case 'country':
        return {
          key: account.country_code ? `country:${account.country_code}` : 'country:none',
          label: accountCountryLabel(account, countryNames),
          rank: account.country_code ? 1 : 99
        };
      case 'kind':
        return {
          key: `kind:${account.account_kind}`,
          label: accountKindLabel(account.account_kind),
          rank: 1
        };
      case 'status':
        return {
          key: `status:${account.status}`,
          label: accountStatusLabel(account.status),
          rank: account.status === 'active' ? 1 : account.status === 'closed' ? 2 : 3
        };
      case 'none':
        return {
          key: 'all',
          label: m.accounts_group_all(),
          rank: 1
        };
      default:
        return {
          key: account.institution_id ? `institution:${account.institution_id}` : 'institution:none',
          label: accountInstitutionLabel(account, institutionsByID),
          rank: account.institution_id ? 1 : 99
        };
    }
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
        <AccountFilterBar bind:query bind:statusFilter bind:classFilter bind:groupMode bind:sortMode />
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

  <section class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
    <div class="min-w-0">
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
          <button
            type="button"
            class="inline-flex items-center gap-2 rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
            onclick={() => (editor = { type: 'account-create' })}
          >
            <WalletCards size={16} aria-hidden="true" />
            {m.accounts_add_account()}
          </button>
        </StatePanel>
      {:else if visibleAccounts.length === 0}
        <StatePanel title={m.accounts_no_results_title()} copy={m.accounts_no_results_copy()} />
      {:else}
        <div class="space-y-4">
          {#each groupedAccounts as group (group.key)}
            <AccountGroupSection
              label={group.label}
              accounts={group.accounts}
              {institutionsByID}
              {currenciesByID}
              {countryNames}
              onEdit={(account) => (editor = { type: 'account-edit', account })}
              onClose={handleCloseAccount}
              onArchive={handleArchiveAccount}
              onDelete={handleDeleteAccount}
            />
          {/each}
        </div>
      {/if}
    </div>

    <Panel>
      <div class="flex items-center justify-between gap-3">
        <h2 class="text-base font-semibold text-foreground">{m.institutions_panel_title()}</h2>
        <button
          type="button"
          class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover"
          onclick={() => (editor = { type: 'institution-create' })}
          aria-label={m.institutions_add_institution()}
          title={m.institutions_add_institution()}
        >
          <Plus size={16} aria-hidden="true" />
        </button>
      </div>

      {#if activeInstitutions.length === 0}
        <p class="mt-3 text-sm leading-6 text-muted">{m.institutions_empty()}</p>
      {:else}
        <div class="mt-4 divide-y divide-border">
          {#each activeInstitutions as institution (institution.id)}
            <article class="flex items-center justify-between gap-3 py-3">
              <div class="min-w-0">
                <p class="truncate text-sm font-semibold text-foreground">{institution.name}</p>
                <p class="mt-1 text-xs text-muted">
                  {m.institutions_account_count({
                    count: institutionAccountCounts.get(institution.id) ?? 0
                  })}
                </p>
              </div>

              <div class="flex items-center gap-1">
                <button
                  type="button"
                  class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-border bg-control text-foreground transition hover:bg-control-hover"
                  onclick={() => (editor = { type: 'institution-edit', institution })}
                  aria-label={m.institutions_action_edit({ name: institution.name })}
                  title={m.institutions_action_edit({ name: institution.name })}
                >
                  <Edit3 size={15} aria-hidden="true" />
                </button>
                <button
                  type="button"
                  class="inline-flex h-9 w-9 items-center justify-center rounded-[var(--radius-control)] border border-danger/30 bg-danger-soft text-danger transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
                  onclick={() => handleDeleteInstitution(institution)}
                  disabled={(institutionAccountCounts.get(institution.id) ?? 0) > 0 || actionPendingKey === `institution-delete:${institution.id}`}
                  aria-label={m.institutions_action_delete({ name: institution.name })}
                  title={m.institutions_action_delete({ name: institution.name })}
                >
                  <Trash2 size={15} aria-hidden="true" />
                </button>
              </div>
            </article>
          {/each}
        </div>
      {/if}
    </Panel>
  </section>
</div>
