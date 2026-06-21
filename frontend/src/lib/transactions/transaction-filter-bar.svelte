<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { m } from '$lib/paraglide/messages.js';
  import { accountsQueryOptions } from '$lib/api/accounts';
  import { payeesQueryOptions } from '$lib/api/payees';
  import type { TransactionListOptions } from '$lib/api/transactions';

  export type TransactionFilters = Pick<
    TransactionListOptions,
    'status' | 'kind' | 'accountID' | 'payeeID' | 'q' | 'needsReview' | 'afterDate' | 'beforeDate'
  >;

  /**
   * Controls which filter controls are visible. Each context passes only what
   * is relevant: the global list shows all; the register shows only status + dates.
   */
  export type FilterVisibility = {
    status?: boolean;
    kind?: boolean;
    account?: boolean;
    payee?: boolean;
    search?: boolean;
    dateRange?: boolean;
    needsReview?: boolean;
  };

  let {
    filters = $bindable<TransactionFilters>({}),
    show = {}
  }: {
    filters: TransactionFilters;
    show?: FilterVisibility;
  } = $props();

  const controlClass =
    'mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent';

  // Accounts are only loaded when the account filter control is visible.
  const accountsQuery = $derived(
    show.account
      ? createQuery(() => accountsQueryOptions(false, false))
      : null
  );

  // Payees are loaded with a debounced search term when the payee filter is visible.
  let payeeSearch = $state('');
  let payeeSearchDebounced = $state('');
  let payeeDebounceTimer: ReturnType<typeof setTimeout> | undefined;

  function onPayeeInput(e: Event) {
    payeeSearch = (e.target as HTMLInputElement).value;
    clearTimeout(payeeDebounceTimer);
    payeeDebounceTimer = setTimeout(() => {
      payeeSearchDebounced = payeeSearch;
    }, 250);
  }

  const payeesQuery = $derived(
    show.payee
      ? createQuery(() => payeesQueryOptions({ q: payeeSearchDebounced || undefined }))
      : null
  );

  const visibleAccounts = $derived(
    accountsQuery?.data?.accounts?.filter((a) => a.status === 'active') ?? []
  );

  const visiblePayees = $derived(payeesQuery?.data?.payees ?? []);
</script>

<div class="flex flex-wrap gap-3">
  {#if show.search}
    <label class="block min-w-[12rem] flex-1">
      <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted"
        >{m.transactions_filter_search()}</span
      >
      <input
        type="search"
        value={filters.q ?? ''}
        oninput={(e) => (filters = { ...filters, q: (e.target as HTMLInputElement).value || undefined })}
        class={controlClass}
        placeholder={m.transactions_filter_search_placeholder()}
      />
    </label>
  {/if}

  {#if show.status}
    <label class="block min-w-[9rem]">
      <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted"
        >{m.transactions_filter_status()}</span
      >
      <select
        value={filters.status ?? ''}
        onchange={(e) => {
          const v = (e.target as HTMLSelectElement).value;
          filters = { ...filters, status: v ? (v as TransactionFilters['status']) : undefined };
        }}
        class={controlClass}
      >
        <option value="">{m.transactions_filter_status_all()}</option>
        <option value="posted">{m.transactions_filter_status_posted()}</option>
        <option value="voided">{m.transactions_filter_status_voided()}</option>
      </select>
    </label>
  {/if}

  {#if show.kind}
    <label class="block min-w-[9rem]">
      <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted"
        >{m.transactions_filter_kind()}</span
      >
      <select
        value={filters.kind ?? ''}
        onchange={(e) => {
          const v = (e.target as HTMLSelectElement).value;
          filters = {
            ...filters,
            kind: v ? (v as TransactionFilters['kind']) : undefined
          };
        }}
        class={controlClass}
      >
        <option value="">{m.transactions_filter_kind_all()}</option>
        <option value="ordinary">{m.transaction_kind_ordinary()}</option>
        <option value="transfer">{m.transaction_kind_transfer()}</option>
        <option value="investment">{m.transaction_kind_investment()}</option>
        <option value="adjustment">{m.transaction_kind_adjustment()}</option>
      </select>
    </label>
  {/if}

  {#if show.account}
    <label class="block min-w-[12rem]">
      <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted"
        >{m.transactions_filter_account()}</span
      >
      <select
        value={filters.accountID ?? ''}
        onchange={(e) => {
          const v = (e.target as HTMLSelectElement).value;
          filters = { ...filters, accountID: v ? Number(v) : undefined };
        }}
        class={controlClass}
      >
        <option value="">{m.transactions_filter_account_all()}</option>
        {#each visibleAccounts as account (account.id)}
          <option value={account.id}>{account.name ?? account.code ?? String(account.id)}</option>
        {/each}
      </select>
    </label>
  {/if}

  {#if show.payee}
    <label class="block min-w-[12rem]">
      <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted"
        >{m.transactions_filter_payee()}</span
      >
      <!-- Simple filterable select: input drives the query; select picks the value. -->
      <div class="relative mt-1.5">
        <input
          type="search"
          value={payeeSearch}
          oninput={onPayeeInput}
          class={controlClass + ' mt-0 pr-8'}
          placeholder={m.transactions_filter_payee_placeholder()}
        />
        {#if visiblePayees.length > 0 && payeeSearch}
          <ul
            class="absolute z-20 mt-1 w-full rounded-(--radius-control) border border-border bg-surface text-sm shadow-(--shadow-panel)"
          >
            <li>
              <button
                type="button"
                class="w-full px-3 py-2 text-left text-muted hover:bg-row-hover"
                onclick={() => {
                  filters = { ...filters, payeeID: undefined };
                  payeeSearch = '';
                  payeeSearchDebounced = '';
                }}
              >
                {m.transactions_filter_account_all()}
              </button>
            </li>
            {#each visiblePayees as payee (payee.id)}
              <li>
                <button
                  type="button"
                  class="w-full px-3 py-2 text-left hover:bg-row-hover"
                  onclick={() => {
                    filters = { ...filters, payeeID: payee.id };
                    payeeSearch = payee.name;
                    payeeSearchDebounced = payee.name;
                  }}
                >
                  {payee.name}
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    </label>
  {/if}

  {#if show.dateRange}
    <label class="block min-w-[9rem]">
      <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted"
        >{m.transactions_filter_date_after()}</span
      >
      <input
        type="date"
        value={filters.afterDate ?? ''}
        onchange={(e) =>
          (filters = { ...filters, afterDate: (e.target as HTMLInputElement).value || undefined })}
        class={controlClass}
      />
    </label>

    <label class="block min-w-[9rem]">
      <span class="text-xs font-semibold uppercase tracking-[0.12em] text-muted"
        >{m.transactions_filter_date_before()}</span
      >
      <input
        type="date"
        value={filters.beforeDate ?? ''}
        onchange={(e) =>
          (filters = { ...filters, beforeDate: (e.target as HTMLInputElement).value || undefined })}
        class={controlClass}
      />
    </label>
  {/if}

  {#if show.needsReview}
    <label class="flex min-w-36 items-center gap-2 self-end pb-2.5">
      <input
        type="checkbox"
        checked={filters.needsReview ?? false}
        onchange={(e) =>
          (filters = {
            ...filters,
            needsReview: (e.target as HTMLInputElement).checked || undefined
          })}
        class="h-4 w-4 rounded border-border accent-accent"
      />
      <span class="text-sm font-medium text-foreground"
        >{m.transactions_filter_needs_review_only()}</span
      >
    </label>
  {/if}
</div>
