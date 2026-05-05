<script lang="ts">
  import { onMount } from "svelte";
  import { listTransactionViews, type TransactionSavedView } from "$lib/api/search";
  import { listTransactionTemplates, type TransactionTemplate } from "$lib/api/templates";
  import * as Card from "$lib/components/ui/card";
  import * as Alert from "$lib/components/ui/alert";
  import { Button } from "$lib/components/ui/button";

  const bookId = 1;
  let views: TransactionSavedView[] = [];
  let templates: TransactionTemplate[] = [];
  let error = "";

  onMount(load);

  async function load() {
    error = "";
    try {
      [views, templates] = await Promise.all([
        listTransactionViews(bookId),
        listTransactionTemplates(bookId),
      ]);
    } catch (e) {
      error = String(e);
    }
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Saved Views & Templates</Card.Title>
    <Card.Description>Review transaction shortcuts for the current book.</Card.Description>
  </Card.Header>
  <Card.Content class="space-y-4">
    {#if error}<Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
    <Button variant="outline" onclick={load}>Refresh</Button>
    <div class="grid gap-4 md:grid-cols-2">
      <div>
        <h3 class="mb-2 font-medium">Saved views</h3>
        <div class="space-y-2">
          {#each views as view}
            <div class="rounded-md border p-3 text-sm">
              <div class="font-medium">{view.name}</div>
              <div class="text-muted-foreground">{view.filters.search ?? "No search"} · {view.filters.status ?? "active"}</div>
            </div>
          {/each}
          {#if views.length === 0}<p class="text-sm text-muted-foreground">No saved views.</p>{/if}
        </div>
      </div>
      <div>
        <h3 class="mb-2 font-medium">Templates</h3>
        <div class="space-y-2">
          {#each templates as template}
            <div class="rounded-md border p-3 text-sm">
              <div class="font-medium">{template.name}</div>
              <div class="text-muted-foreground">{template.memo ?? "No memo"} · {template.splits.length} splits</div>
            </div>
          {/each}
          {#if templates.length === 0}<p class="text-sm text-muted-foreground">No templates.</p>{/if}
        </div>
      </div>
    </div>
  </Card.Content>
</Card.Root>
