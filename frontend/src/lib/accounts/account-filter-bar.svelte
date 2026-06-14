<script lang="ts">
  import { m } from '$lib/paraglide/messages.js';
  import type { ClassFilter, StatusFilter } from './account-list-options';

  let {
    query = $bindable(''),
    statusFilter = $bindable<StatusFilter>('active'),
    classFilter = $bindable<ClassFilter>('all')
  } = $props<{
    query: string;
    statusFilter: StatusFilter;
    classFilter: ClassFilter;
  }>();

  const controlClass =
    'mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent';
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
    <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted">{m.accounts_filter_class()}</span>
    <select bind:value={classFilter} class={controlClass}>
      <option value="all">{m.accounts_filter_class_all()}</option>
      <option value="asset">{m.account_class_asset()}</option>
      <option value="liability">{m.account_class_liability()}</option>
      <option value="equity">{m.account_class_equity()}</option>
      <option value="income">{m.account_class_income()}</option>
      <option value="expense">{m.account_class_expense()}</option>
    </select>
  </label>
</div>
