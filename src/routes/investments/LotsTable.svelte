<script lang="ts">
  import { Badge } from "$lib/components/ui/badge";
  import * as Card from "$lib/components/ui/card";
  import * as Table from "$lib/components/ui/table";
  import type { InvestmentLotHoldingSummary } from "$lib/api/investments";
  import { formatCurrency, formatDate, formatQuantity } from "./forms";

  export let lots: InvestmentLotHoldingSummary[] = [];
</script>

<Card.Root>
  <Card.Content class="pt-6">
    {#if lots.length === 0}
      <p class="text-muted-foreground">No tax lots found.</p>
    {:else}
      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Account</Table.Head>
            <Table.Head>Security</Table.Head>
            <Table.Head>Opened</Table.Head>
            <Table.Head class="text-right">Quantity</Table.Head>
            <Table.Head class="text-right">Cost Basis</Table.Head>
            <Table.Head class="text-right">Days Held</Table.Head>
            <Table.Head>Term</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each lots as lot}
            <Table.Row>
              <Table.Cell>{lot.account_name}</Table.Cell>
              <Table.Cell>{lot.commodity_name}</Table.Cell>
              <Table.Cell>{formatDate(lot.opened_date)}</Table.Cell>
              <Table.Cell class="text-right">{formatQuantity(lot.quantity_minor, lot.commodity_scale)}</Table.Cell>
              <Table.Cell class="text-right">{formatCurrency(lot.cost_basis_minor)}</Table.Cell>
              <Table.Cell class="text-right">{lot.holding_days ?? "—"}</Table.Cell>
              <Table.Cell>
                <Badge variant={lot.is_long_term ? "default" : "secondary"}>
                  {lot.is_long_term ? "Long" : "Short"}
                </Badge>
              </Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}
  </Card.Content>
</Card.Root>