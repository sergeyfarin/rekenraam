<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import * as Table from "$lib/components/ui/table";
  import { formatCurrencyMinor } from "$lib/reports/format";

  type Row = { category_id: number; category_name: string; total_minor: number };

  let { rows, total }: { rows: Row[]; total: number } = $props();

  const sorted = $derived([...rows].sort((a, b) => Math.abs(b.total_minor) - Math.abs(a.total_minor)));
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Spending by Category</Card.Title>
  </Card.Header>
  <Card.Content>
    {#if rows.length === 0}
      <p class="text-muted-foreground">No spending data for selected period.</p>
    {:else}
      <div class="mb-6">
        <Card.Root class="surface-money-negative w-fit">
          <Card.Header class="pb-2">
            <Card.Description>Total Spending</Card.Description>
          </Card.Header>
          <Card.Content>
            <div class="text-2xl font-bold text-money-negative-strong">{formatCurrencyMinor(Math.abs(total))}</div>
          </Card.Content>
        </Card.Root>
      </div>

      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Category</Table.Head>
            <Table.Head class="text-right">Amount</Table.Head>
            <Table.Head class="text-right">% of Total</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each sorted as row}
            <Table.Row>
              <Table.Cell>{row.category_name || "(No category)"}</Table.Cell>
              <Table.Cell class="text-right {row.total_minor < 0 ? 'text-money-negative' : 'text-money-positive'}">
                {formatCurrencyMinor(row.total_minor)}
              </Table.Cell>
              <Table.Cell class="text-right text-muted-foreground">
                {total !== 0 ? ((Math.abs(row.total_minor) / Math.abs(total)) * 100).toFixed(1) : 0}%
              </Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}
  </Card.Content>
</Card.Root>
