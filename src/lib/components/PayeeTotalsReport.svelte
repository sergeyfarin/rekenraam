<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import * as Table from "$lib/components/ui/table";
  import { formatCurrencyMinor } from "$lib/reports/format";

  type Row = { payee_id: number; payee_name: string; total_minor: number };

  let { rows, total }: { rows: Row[]; total: number } = $props();

  const sorted = $derived([...rows].sort((a, b) => Math.abs(b.total_minor) - Math.abs(a.total_minor)));
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Spending by Payee</Card.Title>
  </Card.Header>
  <Card.Content>
    {#if rows.length === 0}
      <p class="text-muted-foreground">No payee data for selected period.</p>
    {:else}
      <div class="mb-6">
        <Card.Root class="w-fit">
          <Card.Header class="pb-2">
            <Card.Description>Total</Card.Description>
          </Card.Header>
          <Card.Content>
            <div class="text-2xl font-bold">{formatCurrencyMinor(total)}</div>
          </Card.Content>
        </Card.Root>
      </div>

      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Payee</Table.Head>
            <Table.Head class="text-right">Amount</Table.Head>
            <Table.Head class="text-right">% of Total</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each sorted as row}
            <Table.Row>
              <Table.Cell>{row.payee_name || "(No payee)"}</Table.Cell>
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
