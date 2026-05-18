<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import type { Currency, FxDailyForm, FxRateDaily } from "./types";

  export let fxRatesDaily: FxRateDaily[] = [];
  export let currencies: Currency[] = [];
  export let busy = false;
  export let fxRateStatus = "";
  export let fxRateError = "";
  export let showFxDailyDialog = false;
  export let newFxDaily: FxDailyForm = { from_currency_id: 0, to_currency_id: 0, rate_date: "", rate: 0, source: "" };
  export let onOpenNew: () => void = () => {};
  export let onCloseDialog: () => void = () => {};
  export let onSave: () => void | Promise<void> = () => {};
  export let onDelete: (rate: FxRateDaily) => void | Promise<void> = () => {};
</script>

<div class="flex justify-between items-center mb-4 gap-4">
  <p class="text-sm text-muted-foreground">Market exchange rates for daily currency conversion.</p>
  <Button onclick={onOpenNew} disabled={busy} size="sm">Add Rate</Button>
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
      <Table.Head>Date</Table.Head>
      <Table.Head>From</Table.Head>
      <Table.Head>To</Table.Head>
      <Table.Head>Rate</Table.Head>
      <Table.Head>Source</Table.Head>
      <Table.Head class="text-right">Actions</Table.Head>
    </Table.Row>
  </Table.Header>
  <Table.Body>
    {#each fxRatesDaily as rate}
      <Table.Row>
        <Table.Cell>{rate.rate_date}</Table.Cell>
        <Table.Cell class="font-mono">{rate.from_currency_symbol || "?"}</Table.Cell>
        <Table.Cell class="font-mono">{rate.to_currency_symbol || "?"}</Table.Cell>
        <Table.Cell class="font-mono">{rate.rate.toFixed(6)}</Table.Cell>
        <Table.Cell class="text-muted-foreground">{rate.source || "—"}</Table.Cell>
        <Table.Cell class="text-right">
          <Button variant="ghost" size="sm" class="text-destructive" onclick={() => onDelete(rate)}>Delete</Button>
        </Table.Cell>
      </Table.Row>
    {:else}
      <Table.Row>
        <Table.Cell colspan={6} class="text-muted-foreground">No daily FX rates found.</Table.Cell>
      </Table.Row>
    {/each}
  </Table.Body>
</Table.Root>

<Dialog.Root bind:open={showFxDailyDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Add Daily FX Rate</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="fx-daily-date">Date</Label>
        <Input id="fx-daily-date" type="date" bind:value={newFxDaily.rate_date} />
      </div>
      <div class="grid gap-2">
        <Label for="fx-daily-from">From Currency</Label>
        <select id="fx-daily-from" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxDaily.from_currency_id}>
          {#each currencies.filter((currency) => currency.is_active) as currency}
            <option value={currency.id}>{currency.display_symbol || ""} {currency.symbol} - {currency.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-daily-to">To Currency</Label>
        <select id="fx-daily-to" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newFxDaily.to_currency_id}>
          {#each currencies.filter((currency) => currency.is_active) as currency}
            <option value={currency.id}>{currency.display_symbol || ""} {currency.symbol} - {currency.name}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="fx-daily-rate">Exchange Rate</Label>
        <Input id="fx-daily-rate" type="number" step="0.000001" bind:value={newFxDaily.rate} />
        <p class="text-sm text-muted-foreground">1 unit of "From" currency = this many units of "To" currency</p>
      </div>
      <div class="grid gap-2">
        <Label for="fx-daily-source">Source (optional)</Label>
        <Input id="fx-daily-source" type="text" bind:value={newFxDaily.source} placeholder="e.g. ECB, XE.com, bank" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={onCloseDialog}>Cancel</Button>
      <Button onclick={onSave} disabled={busy || !newFxDaily.rate_date || !newFxDaily.rate}>Add Rate</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>