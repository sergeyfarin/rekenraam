<script lang="ts">
  import * as Card from "$lib/components/ui/card";
  import type { PortfolioPerformanceSummary } from "$lib/api/reports";
  import { formatCurrencyMinor } from "$lib/reports/format";

  export let summary: PortfolioPerformanceSummary | null = null;
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Portfolio Performance</Card.Title>
  </Card.Header>
  <Card.Content>
    {#if !summary}
      <p class="text-muted-foreground">No portfolio performance data available.</p>
    {:else}
      <div class="grid gap-4 md:grid-cols-3">
        <Card.Root>
          <Card.Header class="pb-2"><Card.Description>Market Value</Card.Description></Card.Header>
          <Card.Content><div class="text-2xl font-bold">{formatCurrencyMinor(summary.market_value_minor)}</div></Card.Content>
        </Card.Root>
        <Card.Root>
          <Card.Header class="pb-2"><Card.Description>Cost Basis</Card.Description></Card.Header>
          <Card.Content><div class="text-2xl font-bold">{formatCurrencyMinor(summary.cost_basis_minor)}</div></Card.Content>
        </Card.Root>
        <Card.Root class={summary.unrealized_gain_minor >= 0 ? "surface-money-positive" : "surface-money-negative"}>
          <Card.Header class="pb-2"><Card.Description>Unrealized Gain/Loss</Card.Description></Card.Header>
          <Card.Content><div class="text-2xl font-bold">{formatCurrencyMinor(summary.unrealized_gain_minor)}</div></Card.Content>
        </Card.Root>
        <Card.Root class={summary.realized_gain_minor >= 0 ? "surface-money-positive" : "surface-money-negative"}>
          <Card.Header class="pb-2"><Card.Description>Realized Gain/Loss</Card.Description></Card.Header>
          <Card.Content><div class="text-2xl font-bold">{formatCurrencyMinor(summary.realized_gain_minor)}</div></Card.Content>
        </Card.Root>
        <Card.Root>
          <Card.Header class="pb-2"><Card.Description>Income</Card.Description></Card.Header>
          <Card.Content><div class="text-2xl font-bold">{formatCurrencyMinor(summary.income_minor)}</div></Card.Content>
        </Card.Root>
        <Card.Root>
          <Card.Header class="pb-2"><Card.Description>Missing Prices</Card.Description></Card.Header>
          <Card.Content><div class="text-2xl font-bold">{summary.price_missing_count}</div></Card.Content>
        </Card.Root>
      </div>
    {/if}
  </Card.Content>
</Card.Root>