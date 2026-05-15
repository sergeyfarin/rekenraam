<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import * as Table from "$lib/components/ui/table";
  import { formatCurrencyMinor, formatPeriod } from "$lib/reports/format";

  type CashflowRow = {
    period_start: string;
    inflow_minor: number;
    outflow_minor: number;
    net_minor: number;
  };

  type Totals = { inflow: number; outflow: number; net: number };

  let { rows, totals, groupBy }: {
    rows: CashflowRow[];
    totals: Totals;
    groupBy: string;
  } = $props();
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Cash Flow Report</Card.Title>
  </Card.Header>
  <Card.Content>
    {#if rows.length === 0}
      <p class="text-muted-foreground">No data for selected period.</p>
    {:else}
      <div class="grid gap-4 md:grid-cols-3 mb-6">
        <Card.Root class="surface-money-positive">
          <Card.Header class="pb-2">
            <Card.Description>Total Inflow</Card.Description>
          </Card.Header>
          <Card.Content>
            <div class="text-2xl font-bold text-money-positive-strong">{formatCurrencyMinor(totals.inflow)}</div>
          </Card.Content>
        </Card.Root>
        <Card.Root class="surface-money-negative">
          <Card.Header class="pb-2">
            <Card.Description>Total Outflow</Card.Description>
          </Card.Header>
          <Card.Content>
            <div class="text-2xl font-bold text-money-negative-strong">{formatCurrencyMinor(totals.outflow)}</div>
          </Card.Content>
        </Card.Root>
        <Card.Root class={totals.net >= 0 ? "surface-money-positive" : "surface-money-negative"}>
          <Card.Header class="pb-2">
            <Card.Description>Net Cash Flow</Card.Description>
          </Card.Header>
          <Card.Content>
            <div class="text-2xl font-bold">{formatCurrencyMinor(totals.net)}</div>
          </Card.Content>
        </Card.Root>
      </div>

      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Period</Table.Head>
            <Table.Head class="text-right">Inflow</Table.Head>
            <Table.Head class="text-right">Outflow</Table.Head>
            <Table.Head class="text-right">Net</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each rows as row}
            <Table.Row>
              <Table.Cell>{formatPeriod(row.period_start, groupBy)}</Table.Cell>
              <Table.Cell class="text-right text-money-positive">{formatCurrencyMinor(row.inflow_minor)}</Table.Cell>
              <Table.Cell class="text-right text-money-negative">{formatCurrencyMinor(row.outflow_minor)}</Table.Cell>
              <Table.Cell class="text-right {row.net_minor >= 0 ? 'text-money-positive' : 'text-money-negative'}">
                {formatCurrencyMinor(row.net_minor)}
              </Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}
  </Card.Content>
</Card.Root>
