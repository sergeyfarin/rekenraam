<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { m } from '$lib/paraglide/messages.js';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import { accountsQueryOptions, type AccountResponse } from '$lib/api/accounts';
  import { currenciesQueryOptions, type CurrencyResponse } from '$lib/api/currencies';
  import { parseStatementBalance } from '$lib/reconcile/statement-balance';
  import {
    startReconciliation,
    type ReconciliationSessionResponse,
    type StartReconciliationRequest
  } from '$lib/api/reconciliation';

  let {
    csrfToken,
    onStarted
  }: {
    csrfToken: string | undefined;
    onStarted: (session: ReconciliationSessionResponse) => void;
  } = $props();

  const inputClass =
    'mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent disabled:cursor-not-allowed disabled:opacity-70';
  const labelClass = 'block text-xs font-semibold uppercase tracking-[0.12em] text-muted';

  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  // Only cash/liability accounts that accept postings can be reconciled.
  const reconcilableAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) =>
        (a.account_class === 'asset' || a.account_class === 'liability') &&
        a.status === 'active' &&
        a.allows_postings
    )
  );

  // currency.id === commodity id (same mapping the transaction editor relies on).
  const currenciesByID = $derived(
    new Map<number, CurrencyResponse>(
      (currenciesQuery.data?.currencies ?? []).map((c: CurrencyResponse) => [c.id, c])
    )
  );

  let accountID = $state<number | undefined>(undefined);
  let statementDate = $state('');
  let balanceStr = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let invalidBalance = $state(false);

  const selectedAccount = $derived(
    accountID !== undefined ? reconcilableAccounts.find((a) => a.id === accountID) : undefined
  );
  const commodityID = $derived(selectedAccount?.default_commodity_id);
  const currency = $derived(commodityID !== undefined ? currenciesByID.get(commodityID) : undefined);

  const canSubmit = $derived(
    !!csrfToken &&
      accountID !== undefined &&
      commodityID !== undefined &&
      statementDate.trim() !== '' &&
      balanceStr.trim() !== '' &&
      !pending
  );

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    invalidBalance = false;
    formError = undefined;

    if (!csrfToken || accountID === undefined || commodityID === undefined) return;

    const parsed = parseStatementBalance(balanceStr);
    if (!parsed) {
      invalidBalance = true;
      return;
    }

    const req: StartReconciliationRequest = {
      commodity_id: commodityID,
      statement_date: statementDate,
      statement_balance_value: parsed.value,
      statement_balance_scale: parsed.scale
    };

    pending = true;
    try {
      const session = await startReconciliation(accountID, req, csrfToken);
      onStarted(session);
    } catch (error) {
      formError = error;
    } finally {
      pending = false;
    }
  }
</script>

<div class="space-y-5">
  <div>
    <h2 class="text-sm font-semibold text-foreground">{m.reconcile_start_title()}</h2>
    <p class="mt-1 text-sm leading-6 text-muted">{m.reconcile_start_copy()}</p>
  </div>

  {#if accountsQuery.isSuccess && reconcilableAccounts.length === 0}
    <p class="text-sm text-muted">{m.reconcile_start_no_accounts()}</p>
  {:else}
    <form class="space-y-4" onsubmit={handleSubmit}>
      <div>
        <label class={labelClass} for="reconcile-account">{m.reconcile_start_account_label()}</label>
        <select id="reconcile-account" class={inputClass} bind:value={accountID} disabled={pending}>
          <option value={undefined} disabled selected={accountID === undefined}>
            {m.reconcile_start_account_placeholder()}
          </option>
          {#each reconcilableAccounts as account (account.id)}
            <option value={account.id}>{account.name ?? account.code ?? account.id}</option>
          {/each}
        </select>
      </div>

      {#if accountID !== undefined && commodityID === undefined}
        <p class="text-sm text-warning">{m.reconcile_start_no_commodity()}</p>
      {/if}

      {#if currency}
        <p class="text-xs text-muted">
          {m.reconcile_start_currency_label()}: <span class="font-semibold text-foreground">{currency.code}</span>
        </p>
      {/if}

      <div>
        <label class={labelClass} for="reconcile-date">{m.reconcile_start_date_label()}</label>
        <input
          id="reconcile-date"
          type="date"
          class={inputClass}
          bind:value={statementDate}
          disabled={pending}
        />
      </div>

      <div>
        <label class={labelClass} for="reconcile-balance">{m.reconcile_start_balance_label()}</label>
        <input
          id="reconcile-balance"
          type="text"
          inputmode="decimal"
          placeholder="0.00"
          class={inputClass}
          bind:value={balanceStr}
          disabled={pending}
        />
        <p class="mt-1.5 text-xs text-muted">{m.reconcile_start_balance_help()}</p>
        {#if invalidBalance}
          <p class="mt-1.5 text-xs text-danger">{m.reconcile_start_invalid_balance()}</p>
        {/if}
      </div>

      <APIFormError error={formError} id="reconcile-start-error" />

      <button
        type="submit"
        class="inline-flex items-center gap-2 rounded-(--radius-control) bg-foreground px-4 py-2.5 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
        disabled={!canSubmit}
      >
        {pending ? m.reconcile_start_submitting() : m.reconcile_start_submit()}
      </button>
    </form>
  {/if}
</div>
