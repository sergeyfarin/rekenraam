<script lang="ts">
  import { onMount } from "svelte";
  import {
    createAccountNote,
    createTransactionNote,
    deleteNote,
    listAccountNotes,
    listTransactionNotes,
    updateNote,
    type MarkdownNote,
  } from "$lib/api/notes";
  import * as Card from "$lib/components/ui/card";
  import * as Alert from "$lib/components/ui/alert";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";

  export let targetType: "account" | "transaction";
  export let targetId: number;

  let notes: MarkdownNote[] = [];
  let title = "";
  let body = "";
  let pinned = false;
  let editingId: number | null = null;
  let error = "";

  onMount(load);
  $: if (targetId) void load();

  async function load() {
    error = "";
    try {
      notes = targetType === "account" ? await listAccountNotes(targetId) : await listTransactionNotes(targetId);
    } catch (e) {
      error = String(e);
    }
  }

  function edit(note: MarkdownNote) {
    editingId = note.id;
    title = note.title;
    body = note.body_markdown;
    pinned = note.pinned;
  }

  function reset() {
    editingId = null;
    title = "";
    body = "";
    pinned = false;
  }

  async function save() {
    error = "";
    try {
      const input = { title, body_markdown: body, pinned };
      if (editingId) {
        await updateNote(editingId, input);
      } else if (targetType === "account") {
        await createAccountNote(targetId, input);
      } else {
        await createTransactionNote(targetId, input);
      }
      reset();
      await load();
    } catch (e) {
      error = String(e);
    }
  }

  async function remove(note: MarkdownNote) {
    await deleteNote(note.id);
    await load();
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Notes</Card.Title>
    <Card.Description>Markdown notes for this {targetType}.</Card.Description>
  </Card.Header>
  <Card.Content class="space-y-4">
    {#if error}<Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
    <div class="grid gap-3 md:grid-cols-[16rem_1fr_auto]">
      <div class="space-y-2">
        <Label for={`note-title-${targetType}-${targetId}`}>Title</Label>
        <Input id={`note-title-${targetType}-${targetId}`} bind:value={title} />
      </div>
      <div class="space-y-2">
        <Label for={`note-body-${targetType}-${targetId}`}>Markdown</Label>
        <Input id={`note-body-${targetType}-${targetId}`} bind:value={body} />
      </div>
      <div class="flex items-end gap-2">
        <label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={pinned} /> Pin</label>
        <Button onclick={save} disabled={!title}>{editingId ? "Update" : "Add"}</Button>
        {#if editingId}<Button variant="ghost" onclick={reset}>Cancel</Button>{/if}
      </div>
    </div>
    <div class="space-y-2">
      {#each notes as note}
        <div class="rounded-md border p-3">
          <div class="flex items-start justify-between gap-2">
            <div>
              <div class="font-medium">{note.pinned ? "Pinned · " : ""}{note.title}</div>
              <p class="whitespace-pre-wrap text-sm text-muted-foreground">{note.body_markdown}</p>
            </div>
            <div class="flex gap-1">
              <Button variant="ghost" size="sm" onclick={() => edit(note)}>Edit</Button>
              <Button variant="ghost" size="sm" onclick={() => remove(note)}>Delete</Button>
            </div>
          </div>
        </div>
      {/each}
      {#if notes.length === 0}
        <p class="text-sm text-muted-foreground">No notes yet.</p>
      {/if}
    </div>
  </Card.Content>
</Card.Root>
