<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { Badge } from "$lib/components/ui/badge";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import { formatOfficialPeriod } from "./types";
  import type { Currency, FxOfficialForm, FxRateOfficial, FxRateSource } from "./types";

  export let fxRatesOfficial: FxRateOfficial[] = [];
  export let currencies: Currency[] = [];
  export let fxRateSources: FxRateSource[] = [];
  export let busy = false;
  export let fxRateStatus = "";
  export let fxRateError = "";
  export let showFxOfficialDialog = false;
  export let newFxOfficial: FxOfficialForm = {
    from_currency_id: 0,
    to_currency_id: 0,
    period_type: "yearly",
    period_year: new Date().getFullYear(),
    period_month: null,
    rate: 0,
    source_name: "",
    source_url: "",
    notes: "",
  };
  export let onOpenNew: () => void = () => {};
  export let onCloseDialog: () => void = () => {};
  export let onSave: () => void | Promise<void> = () => {};
  export let onDelete: (rate: FxRateOfficial) => void | Promise<void> = () => {};
</script>

<div class="flex justify-between items-center mb-4 gap-4">
  <p class="text-sm text-muted-foreground">Official exchange rates from tax authorities for tax reporting purposes.</p>
  <Button onclick={onOpenNew} disabled={busy} size="sm">Add Official Rate</Button>
</div>
{#if fxRateStatus}
  <p class="text-sm text-status-success mb-2">{fxRateStatus}</p>
{/if}
{#if fxRateError}
  <p class="text-sm text-destructive mb-2">{fxRateError}</p>
{/if}

<Table.Root>
  <Table.Header>
    <Table.Row>
      <Table.Head>Period</Table.Head>
      <Table.Head>From</Table.Head>
      <Table.Head>To</Table.Head>
      <Table.Head>Rate</Table.Head>
      <Table.Head>Source</Table.Head>
      <Table.Head class="text-right">Actions</Table.Head>
    </Table.Row>
  </Table.Header>
  <Table.Body>
    {#each fxRatesOfficial as rate}
      <Table.Row>
        <Table.Cell>
          {formatOfficialPeriod(rate)}
          <Badge variant="outline" class="ml-2">{rate.period_type}</Badge>
        </Table.Cell>
        <Table.Cell class="font-mono">{rate.from_currency_symbol || "?"}</Table.Cell>
        <Table.Cell class="font-mono">{rate.to_currency_symbol || "?"}</Table.Cell>
        <Table.Cell class="font-mono">{rate.rate.toFixed(6)}</Table.Cell>
        <Table.Cell>
          {#if rate.source_url}
            <a href={rate.source_url} target="_blank" rel="noopener" class="text-primary hover:underline">{rate.source_name}</a>
          {:else}
            {rate.source_name}
          {/if}
        </Table.Cell>
        <Table.Cell class="text-right">
          <Button variant="ghost" size="sm" class="text-destructive" onclick={() => onDelete(rate)}>Delete</Button>
        </Table.Cell>
      </Table.Row>
    {:else}
      <Table.Row>
        <Table.Cell colspan={6} class="text-muted-foreground">No official FX rates found.</Table.Cell>
      </Table.Row>
    {/each}
  </Table.Body>
</Table.Root>

<Dialog.Root bind:open={showFxOfficialDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Add Official FX Rate</Dialog.Title>
      <p class="text-sm text-muted-foreground">Official rates from tax authorities for tax reporting</p>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid grid-cols-2 gap-4">
        <div class="grid gap-2">
          <Label for="fx-official-type">Period Type</Label>
          <select id="fx-official-type" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.period_type}>
            <option value="yearly">Yearly Average</option>
            <option value="monthly">Monthly Average</option>
          </select>
        </div>
        <div class="grid gap-2">
          <Label for="fx-official-year">Year</Label>
          <Input id="fx-official-year" type="number" bind:value={newFxOfficial.period_year} min="1990" max="2100" />
        </div>
      </div>
      {#if newFxOfficial.period_type === "monthly"}
        <div class="grid gap-2">
          <Label for="fx-official-month">Month</Label>
          <select id="fx-official-month" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.period_month}>
            <option value={1}>January</option>
            <option value={2}>February</option>
            <option value={3}>March</option>
            <option value={4}>April</option>
            <option value={5}>May</option>
            <option value={6}>June</option>
            <option value={7}>July</option>
            <option value={8}>August</option>
            <option value={9}>September</option>
            <option value={10}>October</option>
            <option value={11}>November</option>
            <option value={12}>December</option>
          </select>
        </div>
      {/if}
      <div class="grid gap-2">
        <Label for="fx-official-from">From Currency</Label>
        <select id="fx-official-from" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.from_currency_id}>
          {#each currencies.filter((currency) => currency.is_active) as currency}
            <option value={currency.id}>{currency.display_symbol || ""} {currency.symbol} - {currency.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-official-to">To Currency</Label>
        <select id="fx-official-to" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.to_currency_id}>
          {#each currencies.filter((currency) => currency.is_active) as currency}
            <option value={currency.id}>{currency.display_symbol || ""} {currency.symbol} - {currency.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-official-rate">Exchange Rate</Label>
        <Input id="fx-official-rate" type="number" step="0.000001" bind:value={newFxOfficial.rate} />
      </div>
      <div class="grid gap-2">
        <Label for="fx-official-source">Source Authority</Label>
        <select id="fx-official-source" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxOfficial.source_name}>
          {#each fxRateSources as source}
            <option value={source.name}>{source.name}{source.country_code ? ` (${source.country_code})` : ""}</option>
          {/each}
          <option value="">Other</option>
        </select>
      </div>
      {#if !fxRateSources.find((source) => source.name === newFxOfficial.source_name)}
        <div class="grid gap-2">
          <Label for="fx-official-source-custom">Custom Source Name</Label>
          <Input id="fx-official-source-custom" type="text" bind:value={newFxOfficial.source_name} placeholder="e.g. Tax Authority Name" />
        </div>
      {/if}
      <div class="grid gap-2">
        <Label for="fx-official-url">Source URL (optional)</Label>
        <Input id="fx-official-url" type="url" bind:value={newFxOfficial.source_url} placeholder="https://..." />
      </div>
      <div class="grid gap-2">
        <Label for="fx-official-notes">Notes (optional)</Label>
        <Input id="fx-official-notes" type="text" bind:value={newFxOfficial.notes} placeholder="Additional notes" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={onCloseDialog}>Cancel</Button>
      <Button onclick={onSave} disabled={busy || !newFxOfficial.rate || !newFxOfficial.source_name}>Add Rate</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>