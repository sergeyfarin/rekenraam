<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import * as Table from "$lib/components/ui/table";
  import type { AccountTrendRow } from "$lib/api/reports";
  import { formatCurrencyMinor, formatPeriod } from "$lib/reports/format";

  export let rows: AccountTrendRow[] = [];
  export let groupBy = "month";
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Account Trends</Card.Title>
  </Card.Header>
  <Card.Content>
    {#if rows.length === 0}
      <p class="text-muted-foreground">No account trend data for the selected period.</p>
    {:else}
      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Period</Table.Head>
            <Table.Head>Account</Table.Head>
            <Table.Head class="text-right">Balance</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each rows as row}
            <Table.Row>
              <Table.Cell>{formatPeriod(row.period_start, groupBy)}</Table.Cell>
              <Table.Cell>{row.account_name}</Table.Cell>
              <Table.Cell class="text-right">{formatCurrencyMinor(row.balance_minor)}</Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}
  </Card.Content>
</Card.Root>