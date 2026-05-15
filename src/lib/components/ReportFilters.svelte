<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Card from "$lib/components/ui/card";

  type Commodity = {
    id: number;
    kind: string;
    symbol: string | null;
    name: string;
  };

  let {
    dateFrom = $bindable(""),
    dateTo = $bindable(""),
    groupBy = $bindable("month"),
    selectedCommodityId = $bindable<number | null>(null),
    showGroupBy = false,
    showQuoteCurrency = false,
    commodities = [],
    busy = false,
    onRefresh,
  }: {
    dateFrom?: string;
    dateTo?: string;
    groupBy?: string;
    selectedCommodityId?: number | null;
    showGroupBy?: boolean;
    showQuoteCurrency?: boolean;
    commodities?: Commodity[];
    busy?: boolean;
    onRefresh: () => void | Promise<void>;
  } = $props();

  const currencies = $derived(commodities.filter((c) => c.kind === "currency"));
</script>

<Card.Root>
  <Card.Content class="pt-6">
    <div class="flex flex-wrap items-center gap-4">
      <div class="flex items-center gap-2">
        <Label for="date-from">From:</Label>
        <Input id="date-from" type="date" bind:value={dateFrom} class="w-40" />
      </div>
      <div class="flex items-center gap-2">
        <Label for="date-to">To:</Label>
        <Input id="date-to" type="date" bind:value={dateTo} class="w-40" />
      </div>
      {#if showGroupBy}
        <div class="flex items-center gap-2">
          <Label for="group-by">Group by:</Label>
          <select id="group-by" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={groupBy}>
            <option value="day">Day</option>
            <option value="month">Month</option>
            <option value="quarter">Quarter</option>
            <option value="year">Year</option>
          </select>
        </div>
      {/if}
      {#if showQuoteCurrency}
        <div class="flex items-center gap-2">
          <Label for="quote-currency">Quote currency:</Label>
          <select id="quote-currency" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={selectedCommodityId}>
            {#each currencies as c}
              <option value={c.id}>{c.symbol || c.name}</option>
            {/each}
          </select>
        </div>
      {/if}
      <Button onclick={() => onRefresh()} disabled={busy}>
        {busy ? "Loading..." : "Refresh"}
      </Button>
    </div>
  </Card.Content>
</Card.Root>
