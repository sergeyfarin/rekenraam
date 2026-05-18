<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import * as Table from "$lib/components/ui/table";
  import type { CurrencyExposureRow } from "$lib/api/reports";
  import { formatCurrencyMinor } from "$lib/reports/format";

  export let rows: CurrencyExposureRow[] = [];
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Currency Exposure</Card.Title>
  </Card.Header>
  <Card.Content>
    {#if rows.length === 0}
      <p class="text-muted-foreground">No currency exposure data available.</p>
    {:else}
      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Commodity</Table.Head>
            <Table.Head class="text-right">Native Value</Table.Head>
            <Table.Head class="text-right">Status</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each rows as row}
            <Table.Row>
              <Table.Cell>{row.commodity_name}</Table.Cell>
              <Table.Cell class="text-right">{formatCurrencyMinor(row.native_value_minor)}</Table.Cell>
              <Table.Cell class="text-right text-muted-foreground">{row.price_missing ? "Price missing" : "Ready"}</Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}
  </Card.Content>
</Card.Root>