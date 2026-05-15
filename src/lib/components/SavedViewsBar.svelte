<script lang="ts">
  import { Button } from "$lib/components/ui/button";
  import { Input } from "$lib/components/ui/input";
  import { Label } from "$lib/components/ui/label";
  import type { TransactionSavedView } from "$lib/api/search";
  import type { TransactionTemplate } from "$lib/api/templates";

  let {
    savedViews,
    templates,
    selectedSavedViewId = $bindable<number | null>(null),
    selectedTemplateId = $bindable<number | null>(null),
    newSavedViewName = $bindable(""),
    onApplyView,
    onSaveView,
    onPostTemplate,
  }: {
    savedViews: TransactionSavedView[];
    templates: TransactionTemplate[];
    selectedSavedViewId?: number | null;
    selectedTemplateId?: number | null;
    newSavedViewName?: string;
    onApplyView: () => void | Promise<void>;
    onSaveView: () => void | Promise<void>;
    onPostTemplate: () => void | Promise<void>;
  } = $props();
</script>

<div class="mb-4 flex flex-wrap items-end gap-3 rounded-md border bg-muted/30 p-3">
  <div class="space-y-1">
    <Label for="saved-view-select">Saved view</Label>
    <select id="saved-view-select" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={selectedSavedViewId}>
      <option value={null}>Select view</option>
      {#each savedViews as view}
        <option value={view.id}>{view.name}</option>
      {/each}
    </select>
  </div>
  <Button variant="secondary" size="sm" onclick={() => onApplyView()} disabled={!selectedSavedViewId}>Apply view</Button>
  <div class="space-y-1">
    <Label for="saved-view-name">Save current filters</Label>
    <Input id="saved-view-name" bind:value={newSavedViewName} placeholder="View name" class="w-48" />
  </div>
  <Button variant="outline" size="sm" onclick={() => onSaveView()} disabled={!newSavedViewName.trim()}>Save view</Button>
  <div class="space-y-1">
    <Label for="template-select">Template</Label>
    <select id="template-select" class="flex h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm" bind:value={selectedTemplateId}>
      <option value={null}>Select template</option>
      {#each templates as template}
        <option value={template.id}>{template.name}</option>
      {/each}
    </select>
  </div>
  <Button variant="secondary" size="sm" onclick={() => onPostTemplate()} disabled={!selectedTemplateId}>Post today</Button>
</div>
