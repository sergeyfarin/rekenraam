<script lang="ts">
  import { onMount } from "svelte";
  import { getPreferences, updatePreferences, type UserPreferences } from "$lib/api/preferences";
  import { listBooks, type BookSummary } from "$lib/api/books";
  import * as Card from "$lib/components/ui/card";
  import * as Alert from "$lib/components/ui/alert";
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";

  let preferences: UserPreferences = { default_book_id: null, locale: "", date_format: "iso", number_format: "system", theme: "system" };
  let books: BookSummary[] = [];
  let error = "";
  let status = "";

  onMount(load);

  async function load() {
    try {
      [preferences, books] = await Promise.all([getPreferences(), listBooks()]);
    } catch (e) {
      error = String(e);
    }
  }

  async function save() {
    error = "";
    status = "";
    try {
      preferences = await updatePreferences(preferences);
      status = "Preferences saved.";
    } catch (e) {
      error = String(e);
    }
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Preferences</Card.Title>
    <Card.Description>Set display defaults for your user.</Card.Description>
  </Card.Header>
  <Card.Content class="max-w-xl space-y-4">
    {#if error}<Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
    {#if status}<Alert.Root><Alert.Description>{status}</Alert.Description></Alert.Root>{/if}
    <div class="space-y-2">
      <Label for="pref-book">Default book</Label>
      <select id="pref-book" class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={preferences.default_book_id}>
        <option value={null}>First readable book</option>
        {#each books as book}
          <option value={book.id}>{book.name}</option>
        {/each}
      </select>
    </div>
    <div class="space-y-2">
      <Label for="pref-locale">Locale</Label>
      <Input id="pref-locale" bind:value={preferences.locale} placeholder="en-US" />
    </div>
    <div class="grid gap-3 md:grid-cols-3">
      <div class="space-y-2">
        <Label for="pref-date">Date</Label>
        <Input id="pref-date" bind:value={preferences.date_format} />
      </div>
      <div class="space-y-2">
        <Label for="pref-number">Number</Label>
        <Input id="pref-number" bind:value={preferences.number_format} />
      </div>
      <div class="space-y-2">
        <Label for="pref-theme">Theme</Label>
        <Input id="pref-theme" bind:value={preferences.theme} />
      </div>
    </div>
    <Button onclick={save}>Save</Button>
  </Card.Content>
</Card.Root>
