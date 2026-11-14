<script lang="ts">
  import { page } from '$app/state';
  import { createQuery } from '@tanstack/svelte-query';
  import ArrowLeft from '@lucide/svelte/icons/arrow-left';
  import { m } from '#lib/paraglide/messages.js';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import { isHoldingAccount, nearestBrokerage } from '#lib/accounts/brokerage.ts';
  import { investmentInstrumentsQueryOptions } from '#lib/api/investments.ts';
  import Panel from '#lib/components/panel.svelte';
  import StatePanel from '#lib/components/state-panel.svelte';
  import InstrumentActivity from '#lib/investments/instrument-activity.svelte';
  import InstrumentIncomeSummary from '#lib/investments/instrument-income-summary.svelte';
  import { instrumentPrefill } from '#lib/investments/investment-prefill.ts';
  import TransactionWorkspace, { type TransactionWorkspaceCreate } from '#lib/transactions/transaction-workspace.svelte';

  // An instrument's view (#186): its activity, entered in its context the way
  // a category's transactions are. The route parameter is the commodity ID,
  // the identity postings and positions carry.
  const commodityID = $derived(Number(page.params.id));

  const instrumentsQuery = createQuery(() => investmentInstrumentsQueryOptions());
  const accountsQuery = createQuery(() => accountsQueryOptions(false, false));
  const instrument = $derived(instrumentsQuery.data?.instruments.find((item) => item.commodity_id === commodityID));

  // Each holding of this instrument, with its register and its broker (the
  // nearest brokerage ancestor, as Ledger Overview groups it).
  const holdings = $derived.by(() => {
    const accounts = accountsQuery.data?.accounts ?? [];
    const byID = new Map(accounts.map(account => [account.id, account]));
    return accounts
      .filter(account => account.default_commodity_id === commodityID && isHoldingAccount(account))
      .map(account => ({ account, brokerage: nearestBrokerage(account, byID) }));
  });

  // New transaction opens on its Investment tab with this instrument (#506).
  const create = $derived<TransactionWorkspaceCreate>({
    kind: 'entry',
    defaults: { tab: 'investment', investment: instrumentPrefill(commodityID, accountsQuery.data?.accounts ?? []) }
  });
</script>

{#snippet back()}
  <a
    href="/app/investments"
    class="inline-flex items-center gap-1.5 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
  >
    <ArrowLeft size={14} aria-hidden="true" />
    {m.investments_instrument_view_back()}
  </a>
{/snippet}

{#if instrumentsQuery.isPending}
  <Panel><p role="status" class="text-sm text-muted">{m.investments_instrument_view_loading()}</p></Panel>
{:else if instrumentsQuery.isError}
  <StatePanel alert title={m.investments_error_title()} copy={m.investments_error_copy()} />
{:else if !instrument}
  <div class="space-y-4">
    {@render back()}
    <StatePanel title={m.investments_instrument_view_not_found_title()} copy={m.investments_instrument_view_not_found_copy()} />
  </div>
{:else}
  <TransactionWorkspace {create}>
    {#snippet header()}
      {@render back()}
      <h2 class="text-sm font-semibold text-foreground">{instrument.display_name} ({instrument.symbol || instrument.commodity_code})</h2>
      {#if holdings.length > 0}
        <ul class="flex flex-col gap-x-6 sm:flex-row sm:flex-wrap" aria-label={m.investments_instrument_holdings()}>
          {#each holdings as { account, brokerage } (account.id)}
            <li class="flex min-w-0 flex-wrap items-center gap-x-3">
              <a class="inline-flex min-h-11 items-center text-sm font-semibold underline" href={`/app/ledger/accounts/${account.id}/register`}>{account.name || account.code}</a>
              {#if brokerage}<a class="inline-flex min-h-11 items-center text-sm text-muted underline" href={`/app/ledger/accounts/${brokerage.id}/register`}>{brokerage.name || brokerage.code}</a>{/if}
            </li>
          {/each}
        </ul>
      {/if}
      <InstrumentIncomeSummary {commodityID} />
    {/snippet}
    {#snippet list(actions, csrfToken)}
      <InstrumentActivity {commodityID} {csrfToken} {actions} />
    {/snippet}
  </TransactionWorkspace>
{/if}
