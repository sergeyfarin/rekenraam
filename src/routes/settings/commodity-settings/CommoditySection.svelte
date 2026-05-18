<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import { Badge } from "$lib/components/ui/badge";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import type { Commodity } from "./types";

  export let commodities: Commodity[] = [];
  export let busy = false;
  export let commodityStatus = "";
  export let commodityError = "";
  export let showCommodityDialog = false;
  export let editingCommodity: Commodity | null = null;
  export let onOpenEdit: (commodity: Commodity) => void = () => {};
  export let onCloseDialog: () => void = () => {};
  export let onSave: () => void | Promise<void> = () => {};
</script>

{#if commodityStatus}
  <p class="text-sm text-status-success mb-2">{commodityStatus}</p>
{/if}
{#if commodityError}
  <p class="text-sm text-destructive mb-2">{commodityError}</p>
{/if}

<Table.Root>
  <Table.Header>
    <Table.Row>
      <Table.Head>Symbol</Table.Head>
      <Table.Head>Name</Table.Head>
      <Table.Head>Kind</Table.Head>
      <Table.Head>Scale</Table.Head>
      <Table.Head class="text-right">Actions</Table.Head>
    </Table.Row>
  </Table.Header>
  <Table.Body>
    {#each commodities.filter((commodity) => commodity.kind !== "currency") as commodity}
      <Table.Row>
        <Table.Cell class="font-mono">{commodity.symbol || "—"}</Table.Cell>
        <Table.Cell>{commodity.name}</Table.Cell>
        <Table.Cell>
          <Badge variant="default">{commodity.kind}</Badge>
        </Table.Cell>
        <Table.Cell>{commodity.scale}</Table.Cell>
        <Table.Cell class="text-right">
          <Button variant="ghost" size="sm" onclick={() => onOpenEdit(commodity)}>Edit</Button>
        </Table.Cell>
      </Table.Row>
    {:else}
      <Table.Row>
        <Table.Cell colspan={5} class="text-muted-foreground">No non-currency commodities found.</Table.Cell>
      </Table.Row>
    {/each}
  </Table.Body>
</Table.Root>

<Dialog.Root bind:open={showCommodityDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Edit Commodity</Dialog.Title>
    </Dialog.Header>
    {#if editingCommodity}
      <div class="grid gap-4 py-4">
        <div class="grid gap-2">
          <Label for="commodity-symbol">Symbol</Label>
          <Input id="commodity-symbol" type="text" bind:value={editingCommodity.symbol} placeholder="e.g. USD, EUR, BTC" />
        </div>
        <div class="grid gap-2">
          <Label for="commodity-name">Name</Label>
          <Input id="commodity-name" type="text" bind:value={editingCommodity.name} placeholder="Full name" />
        </div>
        <div class="grid gap-2">
          <Label for="commodity-kind">Kind</Label>
          <Input id="commodity-kind" type="text" value={editingCommodity.kind} disabled />
          <p class="text-sm text-muted-foreground">Kind cannot be changed</p>
        </div>
        <div class="grid gap-2">
          <Label for="commodity-scale">Scale (decimal places)</Label>
          <Input id="commodity-scale" type="number" value={editingCommodity.scale} disabled />
          <p class="text-sm text-muted-foreground">Scale cannot be changed</p>
        </div>
      </div>
    {/if}
    <Dialog.Footer>
      <Button variant="secondary" onclick={onCloseDialog}>Cancel</Button>
      <Button onclick={onSave} disabled={busy || !editingCommodity?.name}>Update</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>