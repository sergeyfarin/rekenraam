<script lang="ts">
  import { invoke } from "@tauri-apps/api/core";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import { Badge } from "$lib/components/ui/badge";

  type Payee = {
    id: number;
    book_id: number;
    name: string;
    kind: string;
    metadata: string | null;
    created_at: string;
    updated_at: string;
  };

  export let payees: Payee[] = [];
  export let busy = false;

  let payeeError = "";
  let payeeStatus = "";
  let editingPayee: Payee | null = null;
  let newPayee = { name: "", kind: "person", metadata: "" };
  let showPayeeDialog = false;

  export async function loadPayees() {
    try {
      payees = await invoke<Payee[]>("list_payees", { bookId: 1 });
    } catch (e) {
      payeeError = `Failed to load payees: ${String(e)}`;
    }
  }

  function openNewPayee() {
    editingPayee = null;
    newPayee = { name: "", kind: "person", metadata: "" };
    showPayeeDialog = true;
  }

  function openEditPayee(p: Payee) {
    editingPayee = p;
    newPayee = { name: p.name, kind: p.kind, metadata: p.metadata || "" };
    showPayeeDialog = true;
  }

  function closePayeeDialog() {
    showPayeeDialog = false;
    editingPayee = null;
  }

  async function savePayee() {
    payeeError = "";
    payeeStatus = "";
    busy = true;
    try {
      if (editingPayee) {
        await invoke("update_payee", {
          payee: {
            id: editingPayee.id,
            book_id: 1,
            name: newPayee.name,
            kind: newPayee.kind,
            metadata: newPayee.metadata || null,
          },
        });
        payeeStatus = "Payee updated.";
      } else {
        await invoke("create_payee", {
          payee: {
            book_id: 1,
            name: newPayee.name,
            kind: newPayee.kind,
            metadata: newPayee.metadata || null,
          },
        });
        payeeStatus = "Payee created.";
      }
      closePayeeDialog();
      await loadPayees();
    } catch (e) {
      payeeError = `Failed to save payee: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function deletePayee(p: Payee) {
    if (!confirm(`Delete payee "${p.name}"?`)) return;
    payeeError = "";
    payeeStatus = "";
    busy = true;
    try {
      await invoke("delete_payee", { payeeId: p.id, bookId: 1 });
      payeeStatus = "Payee deleted.";
      await loadPayees();
    } catch (e) {
      payeeError = `Failed to delete payee: ${String(e)}`;
    } finally {
      busy = false;
    }
  }
</script>

<Card.Root>
  <Card.Header class="flex flex-row items-center justify-between">
    <Card.Title>Payees</Card.Title>
    <Button onclick={openNewPayee} disabled={busy}>
      Add Payee
    </Button>
  </Card.Header>
  <Card.Content>
    {#if payeeStatus}
      <p class="text-sm text-green-600">{payeeStatus}</p>
    {/if}
    {#if payeeError}
      <p class="text-sm text-destructive">{payeeError}</p>
    {/if}

    <Table.Root>
      <Table.Header>
        <Table.Row>
          <Table.Head>Name</Table.Head>
          <Table.Head>Kind</Table.Head>
          <Table.Head>Metadata</Table.Head>
          <Table.Head class="text-right">Actions</Table.Head>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each payees as p}
          <Table.Row>
            <Table.Cell>{p.name}</Table.Cell>
            <Table.Cell><Badge variant="outline">{p.kind}</Badge></Table.Cell>
            <Table.Cell class="text-muted-foreground">{p.metadata || "—"}</Table.Cell>
            <Table.Cell class="text-right">
              <Button variant="ghost" size="sm" onclick={() => openEditPayee(p)}>Edit</Button>
              <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deletePayee(p)}>Delete</Button>
            </Table.Cell>
          </Table.Row>
        {:else}
          <Table.Row>
            <Table.Cell colspan={4} class="text-muted-foreground">No payees found.</Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
  </Card.Content>
</Card.Root>

<!-- Payee Dialog -->
<Dialog.Root bind:open={showPayeeDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>{editingPayee ? "Edit Payee" : "New Payee"}</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="payee-name">Name</Label>
        <Input id="payee-name" type="text" bind:value={newPayee.name} placeholder="Payee name" />
      </div>
      <div class="grid gap-2">
        <Label for="payee-kind">Kind</Label>
        <select id="payee-kind" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newPayee.kind}>
          <option value="person">Person</option>
          <option value="business">Business</option>
          <option value="government">Government</option>
          <option value="other">Other</option>
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="payee-metadata">Metadata (optional)</Label>
        <Input id="payee-metadata" type="text" bind:value={newPayee.metadata} placeholder="Notes or additional info" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closePayeeDialog}>Cancel</Button>
      <Button onclick={savePayee} disabled={busy || !newPayee.name}>
        {editingPayee ? "Update" : "Create"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
