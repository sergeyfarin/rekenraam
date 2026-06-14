<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import { accountKindGroups, accountKindLabel } from './account-labels';
  import type { AccountTypeFilter, StatusFilter } from './account-list-options';

  let {
    query = $bindable(''),
    statusFilter = $bindable<StatusFilter>('active'),
    accountTypeFilter = $bindable<AccountTypeFilter>('all')
  } = $props<{
    query: string;
    statusFilter: StatusFilter;
    accountTypeFilter: AccountTypeFilter;
  }>();

  const controlClass =
    'mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent';
  const accountTypeGroups = $derived(accountKindGroups());
</script>

<div class="grid gap-3 md:grid-cols-[minmax(12rem,1fr)_minmax(9rem,12rem)_minmax(9rem,12rem)]">
  <label class="block">
    <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.accounts_filter_search()}</span>
    <input
      bind:value={query}
      type="search"
      class={controlClass}
      placeholder={m.accounts_filter_search_placeholder()}
    />
  </label>

  <label class="block">
    <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.accounts_filter_status()}</span>
    <select bind:value={statusFilter} class={controlClass}>
      <option value="active">{m.accounts_filter_status_active()}</option>
      <option value="closed">{m.accounts_filter_status_closed()}</option>
      <option value="all">{m.accounts_filter_status_all()}</option>
    </select>
  </label>

  <label class="block">
    <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.accounts_filter_type()}</span>
    <select bind:value={accountTypeFilter} class={controlClass}>
      <option value="all">{m.accounts_filter_type_all()}</option>
      {#each accountTypeGroups as group (group.label)}
        <optgroup label={group.label}>
          {#each group.kinds as kind (kind)}
            <option value={kind}>{accountKindLabel(kind)}</option>
          {/each}
        </optgroup>
      {/each}
    </select>
  </label>
</div>
