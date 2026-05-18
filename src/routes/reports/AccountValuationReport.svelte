<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import * as Table from "$lib/components/ui/table";
  import type { AccountValuationRow } from "$lib/api/reports";
  import { formatCurrencyMinor } from "$lib/reports/format";

  export let rows: AccountValuationRow[] = [];
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Account Valuation</Card.Title>
  </Card.Header>
  <Card.Content>
    {#if rows.length === 0}
      <p class="text-muted-foreground">No account valuation data available.</p>
    {:else}
      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Account</Table.Head>
            <Table.Head class="text-right">Market Value</Table.Head>
            <Table.Head class="text-right">Cost Basis</Table.Head>
            <Table.Head class="text-right">Unrealized Gain/Loss</Table.Head>
            <Table.Head class="text-right">Status</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each rows as row}
            <Table.Row>
              <Table.Cell>{row.account_name}</Table.Cell>
              <Table.Cell class="text-right">{formatCurrencyMinor(row.market_value_minor)}</Table.Cell>
              <Table.Cell class="text-right">{formatCurrencyMinor(row.cost_basis_minor)}</Table.Cell>
              <Table.Cell class="text-right {row.unrealized_gain_minor >= 0 ? 'text-money-positive' : 'text-money-negative'}">
                {formatCurrencyMinor(row.unrealized_gain_minor)}
              </Table.Cell>
              <Table.Cell class="text-right text-muted-foreground">
                {row.price_missing ? "Price missing" : row.fx_missing ? "FX missing" : "Ready"}
              </Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}
  </Card.Content>
</Card.Root>