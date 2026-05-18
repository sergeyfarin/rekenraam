<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import * as Table from "$lib/components/ui/table";
  import { calculateHoldingRow, formatCurrency, formatQuantity, type HoldingsTablePosition } from "./forms";

  export let positions: HoldingsTablePosition[] = [];
</script>

<Card.Root>
  <Card.Content class="pt-6">
    {#if positions.length === 0}
      <p class="text-muted-foreground">No holdings found. Use Buy to add securities.</p>
    {:else}
      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Account</Table.Head>
            <Table.Head>Security</Table.Head>
            <Table.Head class="text-right">Quantity</Table.Head>
            <Table.Head class="text-right">Value</Table.Head>
            <Table.Head class="text-right">Cost Basis</Table.Head>
            <Table.Head class="text-right">Gain/Loss</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each positions as position}
            {@const row = calculateHoldingRow(position)}
            <Table.Row>
              <Table.Cell>{position.account_name}</Table.Cell>
              <Table.Cell>{position.commodity_name}</Table.Cell>
              <Table.Cell class="text-right">{formatQuantity(position.balance_minor, position.commodity_scale)}</Table.Cell>
              <Table.Cell class="text-right">
                {formatCurrency(row.value)}
                {#if row.priceMissing}
                  <span class="text-status-warning" title="Price missing">⚠</span>
                {/if}
              </Table.Cell>
              <Table.Cell class="text-right">{formatCurrency(row.costBasis)}</Table.Cell>
              <Table.Cell class="text-right {row.gainLoss >= 0 ? 'text-money-positive' : 'text-money-negative'}">
                {formatCurrency(row.gainLoss)}
              </Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}
  </Card.Content>
</Card.Root>