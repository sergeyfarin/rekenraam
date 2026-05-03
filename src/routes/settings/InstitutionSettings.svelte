<script lang="ts">
  import { deleteInstitution as deleteInstitutionCommand, listInstitutions, saveInstitutionSettings, type InstitutionSummary } from "$lib/api/metadata";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";
  import { Badge } from "$lib/components/ui/badge";

  type Institution = {
    id: number;
    book_id: number;
    name: string;
    kind: string | null;
    routing: string | null;
    website: string | null;
    metadata: string | null;
    created_at: string;
    updated_at: string;
  };

  export let institutions: Institution[] = [];
  export let busy = false;

  let institutionError = "";
  let institutionStatus = "";
  let editingInstitution: Institution | null = null;
  let newInstitution = { name: "", kind: "", routing: "", website: "", metadata: "" };
  let showInstitutionDialog = false;

  export async function loadInstitutions() {
    try {
      const summaries = await listInstitutions(1);
      institutions = summaries.map((institution: InstitutionSummary) => ({
        id: institution.id,
        book_id: institution.book_id,
        name: institution.name,
        kind: institution.kind,
        routing: institution.routing,
        website: institution.website,
        metadata: institution.metadata,
        created_at: institution.created_at,
        updated_at: institution.updated_at,
      }));
    } catch (e) {
      institutionError = `Failed to load institutions: ${String(e)}`;
    }
  }

  function openNewInstitution() {
    editingInstitution = null;
    newInstitution = { name: "", kind: "", routing: "", website: "", metadata: "" };
    showInstitutionDialog = true;
  }

  function openEditInstitution(inst: Institution) {
    editingInstitution = inst;
    newInstitution = {
      name: inst.name,
      kind: inst.kind || "",
      routing: inst.routing || "",
      website: inst.website || "",
      metadata: inst.metadata || "",
    };
    showInstitutionDialog = true;
  }

  function closeInstitutionDialog() {
    showInstitutionDialog = false;
    editingInstitution = null;
  }

  async function saveInstitution() {
    institutionError = "";
    institutionStatus = "";
    busy = true;
    try {
      if (editingInstitution) {
        await saveInstitutionSettings({
          id: editingInstitution.id,
          book_id: 1,
          name: newInstitution.name,
          kind: newInstitution.kind || null,
          routing: newInstitution.routing || null,
          website: newInstitution.website || null,
          metadata: newInstitution.metadata || null,
        });
        institutionStatus = "Institution updated.";
      } else {
        await saveInstitutionSettings({
          book_id: 1,
          name: newInstitution.name,
          kind: newInstitution.kind || null,
          routing: newInstitution.routing || null,
          website: newInstitution.website || null,
          metadata: newInstitution.metadata || null,
        });
        institutionStatus = "Institution created.";
      }
      closeInstitutionDialog();
      await loadInstitutions();
    } catch (e) {
      institutionError = `Failed to save institution: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteInstitution(inst: Institution) {
    if (!confirm(`Delete institution "${inst.name}"?`)) return;
    institutionError = "";
    institutionStatus = "";
    busy = true;
    try {
      await deleteInstitutionCommand(inst.id, 1);
      institutionStatus = "Institution deleted.";
      await loadInstitutions();
    } catch (e) {
      institutionError = `Failed to delete institution: ${String(e)}`;
    } finally {
      busy = false;
    }
  }
</script>

<Card.Root>
  <Card.Header class="flex flex-row items-center justify-between">
    <Card.Title>Institutions</Card.Title>
    <Button onclick={openNewInstitution} disabled={busy}>
      Add Institution
    </Button>
  </Card.Header>
  <Card.Content>
    {#if institutionStatus}
      <p class="text-sm text-green-600">{institutionStatus}</p>
    {/if}
    {#if institutionError}
      <p class="text-sm text-destructive">{institutionError}</p>
    {/if}

    <Table.Root>
      <Table.Header>
        <Table.Row>
          <Table.Head>Name</Table.Head>
          <Table.Head>Kind</Table.Head>
          <Table.Head>Routing</Table.Head>
          <Table.Head>Website</Table.Head>
          <Table.Head class="text-right">Actions</Table.Head>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each institutions as inst}
          <Table.Row>
            <Table.Cell>{inst.name}</Table.Cell>
            <Table.Cell><Badge variant="outline">{inst.kind || "—"}</Badge></Table.Cell>
            <Table.Cell class="font-mono">{inst.routing || "—"}</Table.Cell>
            <Table.Cell>
              {#if inst.website}
                <a href={inst.website} target="_blank" rel="noopener" class="text-primary hover:underline">{inst.website}</a>
              {:else}
                —
              {/if}
            </Table.Cell>
            <Table.Cell class="text-right">
              <Button variant="ghost" size="sm" onclick={() => openEditInstitution(inst)}>Edit</Button>
              <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deleteInstitution(inst)}>Delete</Button>
            </Table.Cell>
          </Table.Row>
        {:else}
          <Table.Row>
            <Table.Cell colspan={5} class="text-muted-foreground">No institutions found.</Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
  </Card.Content>
</Card.Root>

<!-- Institution Dialog -->
<Dialog.Root bind:open={showInstitutionDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>{editingInstitution ? "Edit Institution" : "New Institution"}</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="institution-name">Name</Label>
        <Input id="institution-name" type="text" bind:value={newInstitution.name} placeholder="Institution name" />
      </div>
      <div class="grid gap-2">
        <Label for="institution-kind">Kind</Label>
        <select id="institution-kind" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" bind:value={newInstitution.kind}>
          <option value="">None</option>
          <option value="bank">Bank</option>
          <option value="credit_union">Credit Union</option>
          <option value="brokerage">Brokerage</option>
          <option value="insurance">Insurance</option>
          <option value="employer">Employer</option>
          <option value="other">Other</option>
        </select>
      </div>
      <div class="grid gap-2">
        <Label for="institution-routing">Routing Number (optional)</Label>
        <Input id="institution-routing" type="text" bind:value={newInstitution.routing} placeholder="e.g. 123456789" />
      </div>
      <div class="grid gap-2">
        <Label for="institution-website">Website (optional)</Label>
        <Input id="institution-website" type="url" bind:value={newInstitution.website} placeholder="https://..." />
      </div>
      <div class="grid gap-2">
        <Label for="institution-metadata">Metadata (optional)</Label>
        <Input id="institution-metadata" type="text" bind:value={newInstitution.metadata} placeholder="Additional notes" />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeInstitutionDialog}>Cancel</Button>
      <Button onclick={saveInstitution} disabled={busy || !newInstitution.name}>
        {editingInstitution ? "Update" : "Create"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
