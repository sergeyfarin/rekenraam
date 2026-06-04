<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import { accountsQueryOptions, type AccountResponse } from '$lib/api/accounts';
  import { currenciesQueryOptions, type CurrencyResponse } from '$lib/api/currencies';
  import { institutionsQueryOptions, type InstitutionResponse } from '$lib/api/institutions';
  import { m } from '$lib/paraglide/messages.js';
  import AccountFilterBar from './account-filter-bar.svelte';
  import AccountGroupSection from './account-group-section.svelte';
  import AccountSummaryStats from './account-summary-stats.svelte';
  import {
    accountClassLabel,
    accountClassRank,
    accountCommodityLabel,
    accountCountryLabel,
    accountDisplayName,
    accountInstitutionLabel,
    accountKindLabel,
    accountStatusLabel,
    type AccountClass
  } from './account-labels';
  import type { ClassFilter, GroupMode, SortMode, StatusFilter } from './account-list-options';

  type AccountGroup = {
    key: string;
    label: string;
    accounts: AccountResponse[];
    rank: number;
  };

  const accountsQuery = createQuery(() => accountsQueryOptions(false));
  const institutionsQuery = createQuery(() => institutionsQueryOptions(false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  let groupMode = $state<GroupMode>('class');
  let sortMode = $state<SortMode>('name');
  let statusFilter = $state<StatusFilter>('active');
  let classFilter = $state<ClassFilter>('all');
  let query = $state('');

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

  const visibleAccounts = $derived.by(() => {
    const normalizedQuery = query.trim().toLocaleLowerCase();

    return userAccounts
      .filter((account) => {
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
  const shellError = $derived(accountsQuery.error ?? institutionsQuery.error ?? currenciesQuery.error);

  const screenState = $derived.by<'loading' | 'error' | 'empty' | 'ready'>(() => {
    if (accountsQuery.isPending || institutionsQuery.isPending || currenciesQuery.isPending) {
      return 'loading';
    }

    if (accountsQuery.isError || institutionsQuery.isError || currenciesQuery.isError) {
      return 'error';
    }

    if (userAccounts.length === 0) {
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
      case 'institution':
        return {
          key: account.institution_id ? `institution:${account.institution_id}` : 'institution:none',
          label: accountInstitutionLabel(account, institutionsByID),
          rank: account.institution_id ? 1 : 99
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
          key: `class:${account.account_class}`,
          label: accountClassLabel(account.account_class),
          rank: accountClassRank(account.account_class)
        };
    }
  }

  async function refreshAccounts() {
    await Promise.all([accountsQuery.refetch(), institutionsQuery.refetch(), currenciesQuery.refetch()]);
  }
</script>

<Panel variant="toolbar">
  <div class="flex flex-col gap-4 xl:flex-row xl:items-end xl:justify-between">
    <AccountSummaryStats {activeCount} {closedCount} {visibleCount} />
    <div class="min-w-0 flex-1">
      <AccountFilterBar bind:query bind:statusFilter bind:classFilter bind:groupMode bind:sortMode />
    </div>
  </div>
</Panel>

{#if screenState === 'loading'}
  <StatePanel title={m.accounts_loading_title()} copy={m.accounts_loading_copy()} />
{:else if screenState === 'error'}
  <StatePanel title={m.accounts_error_title()} copy={m.accounts_error_copy()}>
    <APIFormError error={shellError} id="accounts-error" />
    <button
      type="button"
      class="inline-flex items-center rounded-[var(--radius-control)] bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90"
      onclick={refreshAccounts}
    >
      {m.accounts_retry()}
    </button>
  </StatePanel>
{:else if screenState === 'empty'}
  <StatePanel title={m.accounts_empty_title()} copy={m.accounts_empty_copy()} />
{:else if visibleAccounts.length === 0}
  <StatePanel title={m.accounts_no_results_title()} copy={m.accounts_no_results_copy()} />
{:else}
  <div class="mt-4 space-y-4">
    {#each groupedAccounts as group (group.key)}
      <AccountGroupSection
        label={group.label}
        accounts={group.accounts}
        {institutionsByID}
        {currenciesByID}
        {countryNames}
      />
    {/each}
  </div>
{/if}
