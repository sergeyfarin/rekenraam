<script lang="ts">
  import { isSubledgerManagedAccountKind } from '#lib/accounts/subledger-accounts.ts';
  import { formatEditableAmount } from '#lib/money/amount.ts';
  import { createQuery } from '@tanstack/svelte-query';
  import { m } from '#lib/paraglide/messages.js';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { accountsQueryOptions, type AccountResponse } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions, type CurrencyResponse } from '#lib/api/currencies.ts';
  import { parseStatementBalance } from '#lib/reconcile/statement-balance.ts';
  import { statementCommodityChoice, statementCommodityOptions } from '#lib/reconcile/statement-commodity.ts';
  import {
    accountReconciliationQueryOptions,
    startReconciliation,
    type ReconciliationSessionResponse,
    type StartReconciliationRequest
  } from '#lib/api/reconciliation.ts';

  let {
    csrfToken,
    requestedAccountID,
    requestedCommodityID,
    onStarted,
    onAccountChange
  }: {
    csrfToken: string | undefined;
    // An account named by the page URL (`?account=`), such as a holding's
    // register link. A holding is reconciled only when asked for this way.
    requestedAccountID?: number;
    // The commodity named by the URL (`&commodity=`): the currency Home's
    // Reconciliation card flagged, or a register statement row's commodity.
    // It applies to the requested account; otherwise the account's default.
    requestedCommodityID?: number;
    onStarted: (session: ReconciliationSessionResponse) => void;
    // Lets the page show the selected account's checkpoints beside the form.
    onAccountChange?: (accountID: number | undefined, commodityID: number | undefined) => void;
  } = $props();

  const inputClass =
    'mt-1.5 h-10 w-full rounded-[var(--radius-control)] border border-border bg-control px-3 text-sm text-foreground shadow-sm outline-none transition placeholder:text-muted hover:bg-control-hover focus:border-accent disabled:cursor-not-allowed disabled:opacity-70';
  const labelClass = 'block text-xs font-semibold uppercase tracking-[0.12em] text-muted';

  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const currenciesQuery = createQuery(() => currenciesQueryOptions());

  // Only cash/liability accounts that accept postings can be reconciled.
  // Investment holdings are left out of the default list but stay
  // reconcilable when explicitly requested: a holding statement checkpoint is
  // a legitimate flow the investment commands review (#567 review).
  const reconcilableAccounts = $derived(
    (accountsQuery.data?.accounts ?? []).filter(
      (a: AccountResponse) =>
        (a.account_class === 'asset' || a.account_class === 'liability') &&
        a.status === 'active' &&
        a.allows_postings &&
        (!isSubledgerManagedAccountKind(a.account_kind) || a.id === requestedAccountID)
    )
  );

  // currency.id === commodity id (same mapping the transaction editor relies on).
  const currenciesByID = $derived(
    new Map<number, CurrencyResponse>(
      (currenciesQuery.data?.currencies ?? []).map((c: CurrencyResponse) => [c.id, c])
    )
  );

  let accountID = $state<number | undefined>(undefined);
  // Select the requested account once the list holds it; the owner may still
  // change it afterwards.
  let appliedRequest: number | undefined;
  $effect(() => {
    if (requestedAccountID === undefined || appliedRequest === requestedAccountID) return;
    if (reconcilableAccounts.some((a) => a.id === requestedAccountID)) {
      appliedRequest = requestedAccountID;
      accountID = requestedAccountID;
    }
  });
  let statementDate = $state('');
  let balanceStr = $state('');
  let pending = $state(false);
  let formError = $state<unknown>(undefined);
  let invalidBalance = $state(false);

  const selectedAccount = $derived(
    accountID !== undefined ? reconcilableAccounts.find((a) => a.id === accountID) : undefined
  );
  // A statement covers one commodity. The account's reconciliation lookup,
  // read through that account alone, names every commodity it has activity or
  // a statement in, so an account without a default currency, or with
  // activity in another, can still be reconciled (#594 review). A holding's
  // commodity is its instrument, so it needs no lookup.
  const lookupEnabled = $derived(accountID !== undefined && !isSubledgerManagedAccountKind(selectedAccount?.account_kind));
  const lookupQuery = createQuery(() => ({ ...accountReconciliationQueryOptions(accountID ?? 0), enabled: lookupEnabled }));
  const lookupReady = $derived(!lookupEnabled || lookupQuery.data !== undefined);
  const commodityOptions = $derived.by(() => {
    const defaultID = selectedAccount?.default_commodity_id ?? undefined;
    const defaultCurrency = defaultID !== undefined ? currenciesByID.get(defaultID) : undefined;
    const judged = lookupEnabled && lookupQuery.data?.account_id === accountID ? (lookupQuery.data?.reconciliation?.commodities ?? []) : [];
    return statementCommodityOptions(defaultID !== undefined ? { id: defaultID, code: defaultCurrency?.code ?? '' } : undefined, judged);
  });
  // The working copy's commodity is locked once the form settles on it (or
  // the owner picks one), so a refetch can never move a typed balance to
  // another currency; a locked commodity no longer offered asks for a pick.
  let locked = $state<{ accountID: number; commodityID: number } | undefined>(undefined);
  const lockedForAccount = $derived(locked !== undefined && locked.accountID === accountID ? locked.commodityID : undefined);
  const choice = $derived(
    statementCommodityChoice(
      commodityOptions,
      lockedForAccount,
      accountID !== undefined && accountID === requestedAccountID ? requestedCommodityID : undefined,
      selectedAccount?.default_commodity_id
    )
  );
  $effect(() => {
    if (accountID === undefined || !lookupReady || lockedForAccount !== undefined || choice.commodityID === undefined) return;
    locked = { accountID, commodityID: choice.commodityID };
  });
  const commodityID = $derived(lookupReady ? choice.commodityID : undefined);
  const commodityCode = $derived(commodityOptions.find((option) => option.id === commodityID)?.code ?? '');

  $effect(() => {
    onAccountChange?.(selectedAccount?.id, commodityID);
  });

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

  <!-- Without its accounts the picker would read as an empty book (#329). -->
  <APIFormError error={accountsQuery.error ?? currenciesQuery.error} />
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

      {#if lookupEnabled && lookupQuery.data === undefined && lookupQuery.isError}
        <!-- A failed lookup is not an account without currencies: say so and offer a retry. -->
        <div class="space-y-2">
          <APIFormError error={lookupQuery.error} id="reconcile-commodity-error" />
          <button type="button" class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 text-sm font-semibold transition-colors hover:bg-control-hover" onclick={() => lookupQuery.refetch()}>
            {m.reconcile_session_retry()}
          </button>
        </div>
      {:else if accountID !== undefined && lookupReady && commodityOptions.length === 0}
        <p class="text-sm text-warning">{m.reconcile_start_no_commodity()}</p>
      {/if}

      {#if choice.lost}
        <p class="text-sm text-warning" role="alert">{m.reconcile_start_commodity_lost()}</p>
      {/if}

      {#if commodityOptions.length > 1 || choice.lost}
        <div>
          <label class={labelClass} for="reconcile-commodity">{m.reconcile_start_currency_label()}</label>
          <select
            id="reconcile-commodity"
            class={inputClass}
            value={commodityID ?? ''}
            onchange={(event) => {
              if (accountID !== undefined) locked = { accountID, commodityID: Number(event.currentTarget.value) };
            }}
            disabled={pending}
          >
            {#if commodityID === undefined}
              <option value="" disabled>{m.reconcile_start_currency_placeholder()}</option>
            {/if}
            {#each commodityOptions as option (option.id)}
              <option value={option.id}>{option.code}</option>
            {/each}
          </select>
        </div>
      {:else if commodityCode}
        <p class="text-xs text-muted">
          {m.reconcile_start_currency_label()}: <span class="font-semibold text-foreground">{commodityCode}</span>
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
          placeholder={formatEditableAmount('0', 2)}
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
        class="inline-flex items-center gap-2 rounded-(--radius-control) bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition-colors hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-60"
        disabled={!canSubmit}
      >
        {pending ? m.reconcile_start_submitting() : m.reconcile_start_submit()}
      </button>
    </form>
  {/if}
</div>
