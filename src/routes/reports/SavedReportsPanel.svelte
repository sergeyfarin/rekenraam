<script lang="ts">
  import { Badge } from "$lib/components/ui/badge";
  import { Button } from "$lib/components/ui/button";
  import * as Card from "$lib/components/ui/card";
  import * as Dialog from "$lib/components/ui/dialog";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Table from "$lib/components/ui/table";
  import type { ReportDefinitionSummary, ReportRunSummary } from "$lib/api/reports";
  import type { ReportDefinitionForm } from "./types";

  export let definitions: ReportDefinitionSummary[] = [];
  export let runs: ReportRunSummary[] = [];
  export let busy = false;
  export let status = "";
  export let error = "";
  export let showDefinitionDialog = false;
  export let editingDefinition: ReportDefinitionSummary | null = null;
  export let definitionForm: ReportDefinitionForm;
  export let onOpenNew: () => void = () => {};
  export let onOpenEdit: (definition: ReportDefinitionSummary) => void = () => {};
  export let onCloseDialog: () => void = () => {};
  export let onSave: () => void | Promise<void> = () => {};
  export let onDelete: (definition: ReportDefinitionSummary) => void | Promise<void> = () => {};
</script>

<div class="space-y-6">
  {#if status}
    <p class="text-sm text-status-success">{status}</p>
  {/if}
  {#if error}
    <p class="text-sm text-destructive">{error}</p>
  {/if}

  <Card.Root>
    <Card.Header class="flex flex-row items-center justify-between gap-4">
      <div>
        <Card.Title>Saved Definitions</Card.Title>
        <Card.Description>Persisted custom or builtin report definitions for this book.</Card.Description>
      </div>
      <Button onclick={onOpenNew} disabled={busy}>New Definition</Button>
    </Card.Header>
    <Card.Content>
      {#if definitions.length === 0}
        <p class="text-muted-foreground">No saved report definitions yet.</p>
      {:else}
        <Table.Root>
          <Table.Header>
            <Table.Row>
              <Table.Head>Name</Table.Head>
              <Table.Head>Kind</Table.Head>
              <Table.Head>Query Type</Table.Head>
              <Table.Head class="text-right">Actions</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {#each definitions as definition}
              <Table.Row>
                <Table.Cell>{definition.name}</Table.Cell>
                <Table.Cell><Badge variant="outline">{definition.kind}</Badge></Table.Cell>
                <Table.Cell>{definition.query_type}</Table.Cell>
                <Table.Cell class="text-right space-x-2">
                  <Button variant="ghost" size="sm" onclick={() => onOpenEdit(definition)}>Edit</Button>
                  <Button variant="ghost" size="sm" onclick={() => onDelete(definition)}>Delete</Button>
                </Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      {/if}
    </Card.Content>
  </Card.Root>

  <Card.Root>
    <Card.Header>
      <Card.Title>Saved Runs</Card.Title>
      <Card.Description>Recent persisted report results and pricing context.</Card.Description>
    </Card.Header>
    <Card.Content>
      {#if runs.length === 0}
        <p class="text-muted-foreground">No saved report runs yet.</p>
      {:else}
        <Table.Root>
          <Table.Header>
            <Table.Row>
              <Table.Head>Definition</Table.Head>
              <Table.Head>Pricing Mode</Table.Head>
              <Table.Head>As-of Seq</Table.Head>
              <Table.Head>Created</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {#each runs as run}
              <Table.Row>
                <Table.Cell>{run.definition_id}</Table.Cell>
                <Table.Cell>{run.pricing_mode}</Table.Cell>
                <Table.Cell>{run.as_of_seq}</Table.Cell>
                <Table.Cell>{new Date(run.created_at).toLocaleString()}</Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      {/if}
    </Card.Content>
  </Card.Root>

  <Dialog.Root bind:open={showDefinitionDialog}>
    <Dialog.Content>
      <Dialog.Header>
        <Dialog.Title>{editingDefinition ? "Edit Report Definition" : "New Report Definition"}</Dialog.Title>
      </Dialog.Header>
      <div class="grid gap-4 py-4">
        <div class="grid gap-2">
          <Label for="report-definition-name">Name</Label>
          <Input id="report-definition-name" bind:value={definitionForm.name} />
        </div>
        <div class="grid gap-2 md:grid-cols-2">
          <div class="grid gap-2">
            <Label for="report-definition-kind">Kind</Label>
            <select id="report-definition-kind" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={definitionForm.kind}>
              <option value="custom">Custom</option>
              <option value="builtin">Builtin</option>
            </select>
          </div>
          <div class="grid gap-2">
            <Label for="report-definition-query-type">Query Type</Label>
            <select id="report-definition-query-type" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={definitionForm.query_type}>
              <option value="sql">SQL</option>
              <option value="template">Template</option>
            </select>
          </div>
        </div>
        <div class="grid gap-2">
          <Label for="report-definition-query">Query Text</Label>
          <textarea id="report-definition-query" class="min-h-40 rounded-md border border-input bg-transparent px-3 py-2 text-sm" bind:value={definitionForm.query_text}></textarea>
        </div>
        <div class="grid gap-2">
          <Label for="report-definition-params">Params Schema JSON</Label>
          <Input id="report-definition-params" bind:value={definitionForm.params_schema} placeholder="JSON schema" />
        </div>
      </div>
      <Dialog.Footer>
        <Button variant="secondary" onclick={onCloseDialog}>Cancel</Button>
        <Button onclick={onSave} disabled={busy || !definitionForm.name.trim() || !definitionForm.query_text.trim()}>
          {editingDefinition ? "Update" : "Create"}
        </Button>
      </Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>
</div>