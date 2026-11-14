<script lang="ts">
  import { parseISO } from 'date-fns';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import type { LedgerAccount, LedgerOverview } from '#lib/api/ledger.ts';
  import Panel from '#lib/components/panel.svelte';
  import LedgerValue from './ledger-value.svelte';
  import { accountDisplayName, accountStatusLabel } from './account-labels.ts';
  import type { AccountResponse } from '#lib/api/accounts.ts';
  import { m } from '#lib/paraglide/messages.js';
  let { summary, commodities, asOf }: { summary: LedgerAccount; commodities: LedgerOverview['commodities']; asOf: string } = $props();
  const dateLabel = $derived(new Intl.DateTimeFormat(getLocale(), { year: 'numeric', month: 'short', day: 'numeric' }).format(parseISO(asOf)));
  const code = (id: number) => commodities.find(c => c.commodity_id === id)?.code ?? '';
  const holdings = $derived(new Map(summary.holding_accounts.map(account => [account.id, account])));
  const link = 'inline-flex min-h-11 items-center text-sm font-semibold underline';
  // Every folded child stays reachable, including closed or archived ones and
  // holdings with no units: they still count as the brokerage's dependents,
  // and their registers carry their own lifecycle actions.
  const label = (account: AccountResponse) => account.status === 'active' ? accountDisplayName(account) : `${accountDisplayName(account)} (${accountStatusLabel(account.status)})`;
  // Each position links to its instrument and to its holding's own register,
  // with the holding's status when it is closed or archived.
  const holdingName = (id: number) => { const account = holdings.get(id); return account ? label(account) : String(id); };
  const emptyHoldings = $derived(summary.holding_accounts.filter(account => !summary.positions.some(position => position.holding_account_id === account.id)));
</script>
<Panel>
  <div class="flex flex-wrap items-center justify-between gap-3">
    <h2 class="font-semibold">{m.ledger_brokerage_value({ date: dateLabel })}</h2>
    <LedgerValue value={summary.value} {commodities} />
  </div>
  {#if !summary.value}<p class="mt-3 text-sm text-muted">{summary.unclassified_short ? m.reports_unclassified_short_notice() : m.home_trend_unavailable_copy()}</p>{/if}
  {#if summary.valuation?.rates_used.some(rate => rate.stale)}<p class="mt-3 text-sm text-muted">{m.ledger_outdated()}</p>{/if}
  <h3 class="mt-4 text-sm font-semibold">{m.ledger_cash()}</h3>
  {#each summary.cash as value}<p class="mt-2 text-sm"><LedgerValue {value} {commodities} /></p>{/each}
  {#each summary.cash_accounts as account (account.id)}<a class="mr-3 {link}" href={`/app/ledger/accounts/${account.id}/register`}>{label(account)}</a>{/each}
  {#if summary.cash.length === 0}<p class="mt-2 text-sm text-muted">{m.ledger_no_cash()}</p>{/if}
  {#if summary.other_accounts.length > 0}
    <h3 class="mt-4 text-sm font-semibold">{m.ledger_other_accounts()}</h3>
    {#each summary.other_accounts as account (account.id)}<a class="mr-3 {link}" href={`/app/ledger/accounts/${account.id}/register`}>{label(account)}</a>{/each}
  {/if}
  {#if summary.unsettled.length > 0}
    <p class="mt-3 text-sm text-muted">{m.ledger_unsettled()}</p>
    {#each summary.unsettled as value}<p class="text-sm"><LedgerValue {value} {commodities} /></p>{/each}
  {/if}
  <h3 class="mt-4 text-sm font-semibold">{m.ledger_positions()}</h3>
  {#if summary.positions.length === 0}<p class="mt-2 text-sm text-muted">{m.ledger_no_positions()}</p>{/if}
  <ul class="mt-2 divide-y divide-border">
    {#each summary.positions as position (`${position.holding_account_id}:${position.commodity_id}`)}
      <li class="flex min-w-0 flex-wrap items-center justify-between gap-3 py-3 text-sm">
        <span class="flex min-w-0 flex-wrap items-center gap-x-3">
          <a class={link} href={`/app/investments/instruments/${position.commodity_id}`}>{code(position.commodity_id)}</a>
          <a class="{link} font-normal text-muted" href={`/app/ledger/accounts/${position.holding_account_id}/register`}>{holdingName(position.holding_account_id)}</a>
        </span>
        <LedgerValue value={position.quantity} {commodities} />
        <LedgerValue value={position.value} {commodities} />
      </li>
    {/each}
  </ul>
  {#if emptyHoldings.length > 0}
    <h3 class="mt-4 text-sm font-semibold">{m.ledger_empty_holdings()}</h3>
    {#each emptyHoldings as account (account.id)}<a class="mr-3 {link}" href={`/app/ledger/accounts/${account.id}/register`}>{label(account)}</a>{/each}
  {/if}
</Panel>
