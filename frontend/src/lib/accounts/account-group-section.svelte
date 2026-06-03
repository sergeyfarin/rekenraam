<script lang="ts">
  import type { AccountResponse } from '$lib/api/accounts';
  import type { CurrencyResponse } from '$lib/api/currencies';
  import type { InstitutionResponse } from '$lib/api/institutions';
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

<section class="rounded-[2rem] border border-border/80 bg-surface/95 p-5 shadow-[var(--shadow-panel)] backdrop-blur sm:p-6">
  <div class="flex items-center justify-between gap-4">
    <h4 class="text-lg font-semibold tracking-tight text-foreground">{label}</h4>
    <span class="rounded-full bg-surface-strong px-3 py-1 text-xs font-semibold text-muted">
      {m.accounts_group_count({ count: accounts.length })}
    </span>
  </div>

  <div class="mt-4 divide-y divide-border overflow-hidden rounded-2xl border border-border bg-surface">
    {#each accounts as account (account.id)}
      <article class="grid gap-3 p-4 sm:grid-cols-[minmax(10rem,1fr)_minmax(8rem,0.8fr)_minmax(8rem,0.8fr)_auto] sm:items-center">
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
          <span
            class:bg-danger-soft={account.status === 'closed'}
            class:text-danger={account.status === 'closed'}
            class:status-accent-soft={account.status === 'active'}
            class:text-accent={account.status === 'active'}
            class="rounded-full px-3 py-1 text-xs font-semibold"
          >
            {accountStatusLabel(account.status)}
          </span>
        </div>
      </article>
    {/each}
  </div>
</section>
