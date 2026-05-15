<script lang="ts">
  import type { AccountSummary, AccountBalancingSummary, AccountDirectiveSummary } from "$lib/api/accounts";
  import type { CommoditySummary } from "$lib/api/metadata";
  import { formatMinorWithScale } from "$lib/money";

  let {
    account,
    balanceMinor,
    lastBalancing,
    directives,
    commodities,
    bookingPolicy = $bindable("fifo"),
    savingPolicy,
    onSaveBookingPolicy,
  }: {
    account: AccountSummary;
    balanceMinor: number;
    lastBalancing: AccountBalancingSummary | null;
    directives: AccountDirectiveSummary[];
    commodities: CommoditySummary[];
    bookingPolicy?: string;
    savingPolicy: boolean;
    onSaveBookingPolicy: () => void | Promise<void>;
  } = $props();

  function formatMinor(amountMinor: number, commodityId: number): string {
    const commodity = commodities.find((c) => c.id === commodityId);
    if (!commodity) return String(amountMinor);
    return `${formatMinorWithScale(amountMinor, commodity.scale)} ${commodity.symbol ?? commodity.name}`;
  }

  function findDirectiveDate(kind: string): string | null {
    const matches = directives.filter((d) => d.directive_type === kind);
    if (matches.length === 0) return null;
    return matches[matches.length - 1].directive_date;
  }

  const openedDate = $derived(findDirectiveDate("open"));
  const closedDate = $derived(findDirectiveDate("close"));
</script>

<div class="card account-summary">
  <h1 class="page-title">{account.name}</h1>
  <p class="page-subtitle">
    {account.account_type}
    {#if account.institution_name}
      · {account.institution_name}
    {/if}
    {#if account.country_name}
      · {account.country_name}
    {/if}
    {#if account.number_last4}
      · ••••{account.number_last4}
    {/if}
    {#if account.is_closed}
      · Closed
    {/if}
  </p>
  <div class="account-meta">
    <span>Balance: {formatMinor(balanceMinor, account.commodity_id)}</span>
    {#if lastBalancing}
      <span>
        Last reconciled: {lastBalancing.as_of_date}
        · {formatMinor(lastBalancing.balance_minor, account.commodity_id)}
      </span>
    {/if}
    {#if openedDate}
      <span>Opened: {openedDate}</span>
    {/if}
    {#if closedDate}
      <span>Closed: {closedDate}</span>
    {/if}
  </div>
  {#if account.account_type === "investment"}
    <div class="booking-policy">
      <label class="label" for="booking-policy">Booking policy</label>
      <select
        id="booking-policy"
        class="select"
        bind:value={bookingPolicy}
        onchange={() => onSaveBookingPolicy()}
        disabled={savingPolicy}
      >
        <option value="fifo">FIFO</option>
        <option value="lifo">LIFO</option>
        <option value="average">Average</option>
        <option value="strict">Strict</option>
      </select>
    </div>
  {/if}
</div>

<style>
  .account-summary {
    margin-bottom: 1.5rem;
  }

  .account-meta {
    margin-top: 0.5rem;
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    font-size: 0.875rem;
    color: var(--muted-foreground);
  }

  .booking-policy {
    margin-top: 1rem;
    max-width: 240px;
  }
</style>
