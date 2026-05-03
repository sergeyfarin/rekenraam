<script lang="ts">
  import { createTag, deleteTag as deleteTagCommand, listTags, type TagSummary, updateTag } from "$lib/api/metadata";
  import * as Card from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import * as Dialog from "$lib/components/ui/dialog";
  import * as Table from "$lib/components/ui/table";

  export let tags: TagSummary[] = [];
  export let busy = false;

  let tagError = "";
  let tagStatus = "";
  let editingTag: TagSummary | null = null;
  let newTag = { name: "", color: "" };
  let showTagDialog = false;

  export async function loadTags() {
    try {
      tags = await listTags(1);
    } catch (e) {
      tagError = `Failed to load tags: ${String(e)}`;
    }
  }

  function openNewTag() {
    editingTag = null;
    newTag = { name: "", color: "" };
    showTagDialog = true;
  }

  function openEditTag(t: TagSummary) {
    editingTag = t;
    newTag = { name: t.name, color: t.color || "" };
    showTagDialog = true;
  }

  function closeTagDialog() {
    showTagDialog = false;
    editingTag = null;
  }

  async function saveTag() {
    tagError = "";
    tagStatus = "";
    busy = true;
    try {
      if (editingTag) {
        await updateTag({
          id: editingTag.id,
          book_id: 1,
          name: newTag.name,
          color: newTag.color || null,
        });
        tagStatus = "Tag updated.";
      } else {
        await createTag({
          book_id: 1,
          name: newTag.name,
          color: newTag.color || null,
        });
        tagStatus = "Tag created.";
      }
      closeTagDialog();
      await loadTags();
    } catch (e) {
      tagError = `Failed to save tag: ${String(e)}`;
    } finally {
      busy = false;
    }
  }

  async function deleteTag(t: TagSummary) {
    if (!confirm(`Delete tag "${t.name}"?`)) return;
    tagError = "";
    tagStatus = "";
    busy = true;
    try {
      await deleteTagCommand(t.id, 1);
      tagStatus = "Tag deleted.";
      await loadTags();
    } catch (e) {
      tagError = `Failed to delete tag: ${String(e)}`;
    } finally {
      busy = false;
    }
  }
</script>

<Card.Root>
  <Card.Header class="flex flex-row items-center justify-between">
    <Card.Title>Tags</Card.Title>
    <Button onclick={openNewTag} disabled={busy}>
      Add Tag
    </Button>
  </Card.Header>
  <Card.Content>
    {#if tagStatus}
      <p class="text-sm text-green-600">{tagStatus}</p>
    {/if}
    {#if tagError}
      <p class="text-sm text-destructive">{tagError}</p>
    {/if}

    <Table.Root>
      <Table.Header>
        <Table.Row>
          <Table.Head>Name</Table.Head>
          <Table.Head>Color</Table.Head>
          <Table.Head class="text-right">Actions</Table.Head>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each tags as t}
          <Table.Row>
            <Table.Cell>{t.name}</Table.Cell>
            <Table.Cell>
              {#if t.color}
                <span class="inline-block w-6 h-6 rounded border" style="background-color: {t.color}"></span>
              {:else}
                —
              {/if}
            </Table.Cell>
            <Table.Cell class="text-right">
              <Button variant="ghost" size="sm" onclick={() => openEditTag(t)}>Edit</Button>
              <Button variant="ghost" size="sm" class="text-destructive" onclick={() => deleteTag(t)}>Delete</Button>
            </Table.Cell>
          </Table.Row>
        {:else}
          <Table.Row>
            <Table.Cell colspan={3} class="text-muted-foreground">No tags found.</Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
  </Card.Content>
</Card.Root>

<!-- Tag Dialog -->
<Dialog.Root bind:open={showTagDialog}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>{editingTag ? "Edit Tag" : "New Tag"}</Dialog.Title>
    </Dialog.Header>
    <div class="grid gap-4 py-4">
      <div class="grid gap-2">
        <Label for="tag-name">Name</Label>
        <Input id="tag-name" type="text" bind:value={newTag.name} placeholder="Tag name" />
      </div>
      <div class="grid gap-2">
        <Label for="tag-color">Color</Label>
        <Input id="tag-color" type="color" bind:value={newTag.color} />
      </div>
    </div>
    <Dialog.Footer>
      <Button variant="secondary" onclick={closeTagDialog}>Cancel</Button>
      <Button onclick={saveTag} disabled={busy || !newTag.name}>
        {editingTag ? "Update" : "Create"}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
