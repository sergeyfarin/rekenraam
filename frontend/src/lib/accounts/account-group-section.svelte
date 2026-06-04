<script lang="ts">
  import type { AccountResponse } from '$lib/api/accounts';
  import type { CurrencyResponse } from '$lib/api/currencies';
  import type { InstitutionResponse } from '$lib/api/institutions';
  import StatusBadge from '$lib/components/status-badge.svelte';
  import { m } from '$lib/paraglide/messages.js';
  import {
    accountClassLabel,
    accountCommodityLabel,
    accountCountryLabel,
    accountDisplayName,
    accountInstitutionLabel,
    accountKindLabel,
    accountStatusLabel
  } from './account-labels';

  let { label, accounts, institutionsByID, currenciesByID, countryNames } = $props<{
    label: string;
    accounts: AccountResponse[];
    institutionsByID: ReadonlyMap<number, InstitutionResponse>;
    currenciesByID: ReadonlyMap<number, CurrencyResponse>;
    countryNames: Intl.DisplayNames;
  }>();
</script>

<section class="overflow-hidden rounded-[var(--radius-panel)] border border-border bg-surface shadow-[var(--shadow-panel)]">
  <div class="flex items-center justify-between gap-4 border-b border-border bg-toolbar px-4 py-3">
    <h2 class="truncate text-sm font-semibold tracking-tight text-foreground">{label}</h2>
    <span class="rounded-[var(--radius-control)] bg-surface-strong px-2.5 py-1 text-xs font-semibold text-muted">
      {m.accounts_group_count({ count: accounts.length })}
    </span>
  </div>

  <div class="divide-y divide-border">
    {#each accounts as account (account.id)}
      <article class="grid gap-3 px-4 py-3 transition hover:bg-row-hover sm:grid-cols-[minmax(10rem,1fr)_minmax(8rem,0.8fr)_minmax(8rem,0.8fr)_auto] sm:items-center">
        <div class="min-w-0">
          <p class="truncate text-sm font-semibold text-foreground">{accountDisplayName(account)}</p>
          <p class="mt-1 text-xs text-muted">
            {accountKindLabel(account.account_kind)}
            {#if account.code}
              <span aria-hidden="true"> · </span>{account.code}
            {/if}
          </p>
        </div>

        <div class="min-w-0 text-sm">
          <p class="truncate text-foreground">{accountInstitutionLabel(account, institutionsByID)}</p>
          <p class="mt-1 truncate text-xs text-muted">{accountCountryLabel(account, countryNames)}</p>
        </div>

        <div class="min-w-0 text-sm">
          <p class="truncate text-foreground">{accountCommodityLabel(account, currenciesByID)}</p>
          {#if account.number_last4}
            <p class="mt-1 truncate text-xs text-muted">{m.accounts_number_hint({ last4: account.number_last4 })}</p>
          {:else}
            <p class="mt-1 truncate text-xs text-muted">{accountClassLabel(account.account_class)}</p>
          {/if}
        </div>

        <div class="flex items-center justify-start sm:justify-end">
          <StatusBadge tone={account.status === 'closed' ? 'danger' : 'accent'}>
            {accountStatusLabel(account.status)}
          </StatusBadge>
        </div>
      </article>
    {/each}
  </div>
</section>
