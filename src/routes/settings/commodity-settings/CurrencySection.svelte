<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { Badge } from "$lib/components/ui/badge";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import type { Currency, CurrencyForm } from "./types";

  export let sortedCurrencies: Currency[] = [];
  export let busy = false;
  export let currencyStatus = "";
  export let currencyError = "";
  export let showCurrencyDialog = false;
  export let editingCurrency: Currency | null = null;
  export let newCurrency: CurrencyForm = { symbol: "", display_symbol: "", name: "", scale: 2 };
  export let onOpenNew: () => void = () => {};
  export let onOpenEdit: (currency: Currency) => void = () => {};
  export let onToggleActive: (currency: Currency) => void | Promise<void> = () => {};
  export let onSetDefault: (currency: Currency) => void | Promise<void> = () => {};
  export let onCloseDialog: () => void = () => {};
  export let onSave: () => void | Promise<void> = () => {};
</script>

<div class="flex justify-between items-center mb-4 gap-4">
  <p class="text-sm text-muted-foreground">Manage active currencies and set your default currency. Inactive currencies stay visible here but are excluded from FX entry and refresh flows. The default currency always remains active.</p>
  <Button onclick={onOpenNew} disabled={busy} size="sm">Add Currency</Button>
</div>
{#if currencyStatus}
  <p class="text-sm text-status-success mb-2">{currencyStatus}</p>
{/if}
{#if currencyError}
  <p class="text-sm text-destructive mb-2">{currencyError}</p>
{/if}

<Table.Root>
  <Table.Header>
    <Table.Row>
      <Table.Head>Code</Table.Head>
      <Table.Head>Symbol</Table.Head>
      <Table.Head>Name</Table.Head>
      <Table.Head>Scale</Table.Head>
      <Table.Head>Status</Table.Head>
      <Table.Head class="text-right">Actions</Table.Head>
    </Table.Row>
  </Table.Header>
  <Table.Body>
    {#each sortedCurrencies as currency}
      <Table.Row class={!currency.is_active ? "opacity-50" : ""}>
        <Table.Cell class="font-mono font-semibold">{currency.symbol || "—"}</Table.Cell>
        <Table.Cell class="text-lg">{currency.display_symbol || "—"}</Table.Cell>
        <Table.Cell>{currency.name}</Table.Cell>
        <Table.Cell>{currency.scale}</Table.Cell>
        <Table.Cell>
          {#if currency.is_default}
            <Badge variant="default">Default</Badge>
          {:else if currency.is_active}
            <Badge variant="secondary">Active</Badge>
          {:else}
            <Badge variant="outline">Inactive</Badge>
          {/if}
        </Table.Cell>
        <Table.Cell class="text-right">
          <Button variant="ghost" size="sm" onclick={() => onOpenEdit(currency)}>Edit</Button>
          {#if !currency.is_default}
            <Button variant="ghost" size="sm" onclick={() => onToggleActive(currency)}>{currency.is_active ? "Deactivate" : "Activate"}</Button>
          {/if}
          {#if !currency.is_default}
            <Button variant="ghost" size="sm" onclick={() => onSetDefault(currency)}>Set Default</Button>
          {/if}
        </Table.Cell>
      </Table.Row>
    {:else}
      <Table.Row>
        <Table.Cell colspan={6} class="text-muted-foreground">No currencies found.</Table.Cell>
      </Table.Row>
    {/each}
  </Table.Body>
</Table.Root>

<Dialog.Root bind:open={showCurrencyDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>{editingCurrency ? "Edit Currency" : "Add Currency"}</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid grid-cols-2 gap-4">
        <div class="grid gap-2">
          <Label for="currency-symbol">Code (ISO)</Label>
          <Input id="currency-symbol" type="text" bind:value={newCurrency.symbol} placeholder="e.g. EUR, GBP, JPY" maxlength={10} />
          <p class="text-sm text-muted-foreground">ISO 4217 code</p>
        </div>
        <div class="grid gap-2">
          <Label for="currency-display-symbol">Display Symbol</Label>
          <Input id="currency-display-symbol" type="text" bind:value={newCurrency.display_symbol} placeholder="e.g. €, £, ¥, $" maxlength={5} />
          <p class="text-sm text-muted-foreground">Unicode symbol</p>
        </div>
      </div>
      <div class="grid gap-2">
        <Label for="currency-name">Name</Label>
        <Input id="currency-name" type="text" bind:value={newCurrency.name} placeholder="e.g. Euro, British Pound" />
      </div>
      <div class="grid gap-2">
        <Label for="currency-scale">Decimal Places</Label>
        <select id="currency-scale" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newCurrency.scale}>
          <option value={0}>0 (JPY, KRW, etc.)</option>
          <option value={2}>2 (Most currencies)</option>
          <option value={3}>3 (BHD, KWD, etc.)</option>
        </select>
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={onCloseDialog}>Cancel</Button>
      <Button onclick={onSave} disabled={busy || !newCurrency.symbol || !newCurrency.name}>
        {editingCurrency ? "Update" : "Create"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>