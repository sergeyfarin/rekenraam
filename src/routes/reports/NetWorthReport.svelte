<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import * as Table from "$lib/components/ui/table";
  import type { NetWorthRow } from "$lib/api/reports";
  import { formatCurrencyMinor, formatPeriod } from "$lib/reports/format";

  export let rows: NetWorthRow[] = [];
  export let groupBy = "month";

  $: latest = rows[rows.length - 1] ?? null;
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Net Worth</Card.Title>
  </Card.Header>
  <Card.Content>
    {#if rows.length === 0}
      <p class="text-muted-foreground">No net worth data for the selected period.</p>
    {:else}
      {#if latest}
        <div class="grid gap-4 mb-6 md:grid-cols-3">
          <Card.Root>
            <Card.Header class="pb-2">
              <Card.Description>Assets</Card.Description>
            </Card.Header>
            <Card.Content>
              <div class="text-2xl font-bold">{formatCurrencyMinor(latest.assets_minor)}</div>
            </Card.Content>
          </Card.Root>
          <Card.Root>
            <Card.Header class="pb-2">
              <Card.Description>Liabilities</Card.Description>
            </Card.Header>
            <Card.Content>
              <div class="text-2xl font-bold">{formatCurrencyMinor(latest.liabilities_minor)}</div>
            </Card.Content>
          </Card.Root>
          <Card.Root class={latest.net_worth_minor >= 0 ? "surface-money-positive" : "surface-money-negative"}>
            <Card.Header class="pb-2">
              <Card.Description>Net Worth</Card.Description>
            </Card.Header>
            <Card.Content>
              <div class="text-2xl font-bold">{formatCurrencyMinor(latest.net_worth_minor)}</div>
            </Card.Content>
          </Card.Root>
        </div>
      {/if}

      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Period</Table.Head>
            <Table.Head class="text-right">Assets</Table.Head>
            <Table.Head class="text-right">Liabilities</Table.Head>
            <Table.Head class="text-right">Net Worth</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each rows as row}
            <Table.Row>
              <Table.Cell>{formatPeriod(row.period_start, groupBy)}</Table.Cell>
              <Table.Cell class="text-right">{formatCurrencyMinor(row.assets_minor)}</Table.Cell>
              <Table.Cell class="text-right">{formatCurrencyMinor(row.liabilities_minor)}</Table.Cell>
              <Table.Cell class="text-right {row.net_worth_minor >= 0 ? 'text-money-positive' : 'text-money-negative'}">
                {formatCurrencyMinor(row.net_worth_minor)}
              </Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}
  </Card.Content>
</Card.Root>