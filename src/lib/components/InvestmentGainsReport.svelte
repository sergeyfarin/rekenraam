<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import * as Table from "$lib/components/ui/table";
  import { formatCurrencyMinor, formatDate } from "$lib/reports/format";

  type RealizedRow = {
    tx_id: number;
    txn_date: string;
    commodity_id: number;
    quantity_minor: number;
    proceeds_minor: number;
    quote_commodity_id: number | null;
    cost_basis_minor: number;
    gain_loss_minor: number;
    proceeds_missing: boolean;
  };

  type UnrealizedRow = {
    account_id: number;
    account_name: string;
    account_type: string;
    commodity_id: number;
    commodity_name: string;
    value_minor: number;
    cost_basis_minor: number;
    unrealized_gain_minor: number;
    price_missing: boolean;
  };

  let {
    realized,
    realizedTotal,
    unrealized,
    unrealizedTotal,
  }: {
    realized: RealizedRow[];
    realizedTotal: number;
    unrealized: UnrealizedRow[];
    unrealizedTotal: number;
  } = $props();
</script>

<div class="space-y-6">
  <Card.Root>
    <Card.Header>
      <Card.Title>Realized Gains/Losses</Card.Title>
    </Card.Header>
    <Card.Content>
      {#if realized.length === 0}
        <p class="text-muted-foreground">No realized gains for selected period.</p>
      {:else}
        <div class="mb-6">
          <Card.Root class={realizedTotal >= 0 ? "surface-money-positive w-fit" : "surface-money-negative w-fit"}>
            <Card.Header class="pb-2">
              <Card.Description>Total Realized Gain/Loss</Card.Description>
            </Card.Header>
            <Card.Content>
              <div class="text-2xl font-bold">{formatCurrencyMinor(realizedTotal)}</div>
            </Card.Content>
          </Card.Root>
        </div>

        <Table.Root>
          <Table.Header>
            <Table.Row>
              <Table.Head>Date</Table.Head>
              <Table.Head class="text-right">Qty</Table.Head>
              <Table.Head class="text-right">Proceeds</Table.Head>
              <Table.Head class="text-right">Cost Basis</Table.Head>
              <Table.Head class="text-right">Gain/Loss</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {#each realized as row}
              <Table.Row>
                <Table.Cell>{formatDate(row.txn_date)}</Table.Cell>
                <Table.Cell class="text-right">{(row.quantity_minor / 1000000).toFixed(4)}</Table.Cell>
                <Table.Cell class="text-right">{formatCurrencyMinor(row.proceeds_minor)}</Table.Cell>
                <Table.Cell class="text-right">{formatCurrencyMinor(row.cost_basis_minor)}</Table.Cell>
                <Table.Cell class="text-right {row.gain_loss_minor >= 0 ? 'text-money-positive' : 'text-money-negative'}">
                  {formatCurrencyMinor(row.gain_loss_minor)}
                </Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      {/if}
    </Card.Content>
  </Card.Root>

  <Card.Root>
    <Card.Header>
      <Card.Title>Unrealized Gains/Losses</Card.Title>
    </Card.Header>
    <Card.Content>
      {#if unrealized.length === 0}
        <p class="text-muted-foreground">No unrealized gains data available.</p>
      {:else}
        <div class="mb-6">
          <Card.Root class={unrealizedTotal >= 0 ? "surface-money-positive w-fit" : "surface-money-negative w-fit"}>
            <Card.Header class="pb-2">
              <Card.Description>Total Unrealized Gain/Loss</Card.Description>
            </Card.Header>
            <Card.Content>
              <div class="text-2xl font-bold">{formatCurrencyMinor(unrealizedTotal)}</div>
            </Card.Content>
          </Card.Root>
        </div>

        <Table.Root>
          <Table.Header>
            <Table.Row>
              <Table.Head>Account</Table.Head>
              <Table.Head>Commodity</Table.Head>
              <Table.Head class="text-right">Value</Table.Head>
              <Table.Head class="text-right">Cost Basis</Table.Head>
              <Table.Head class="text-right">Gain/Loss</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {#each unrealized as row}
              <Table.Row>
                <Table.Cell>{row.account_name}</Table.Cell>
                <Table.Cell>{row.commodity_name}</Table.Cell>
                <Table.Cell class="text-right">{formatCurrencyMinor(row.value_minor)}</Table.Cell>
                <Table.Cell class="text-right">{formatCurrencyMinor(row.cost_basis_minor)}</Table.Cell>
                <Table.Cell class="text-right {row.unrealized_gain_minor >= 0 ? 'text-money-positive' : 'text-money-negative'}">
                  {formatCurrencyMinor(row.unrealized_gain_minor)}
                  {#if row.price_missing}
                    <span class="text-status-warning" title="Price missing">⚠</span>
                  {/if}
                </Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      {/if}
    </Card.Content>
  </Card.Root>
</div>
