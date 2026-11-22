<script lang="ts">
  import { page } from '$app/state';
  import CategoryListScreen from '#lib/categories/category-list-screen.svelte';
  import { m } from '#lib/paraglide/messages.js';
  import TagsScreen from '#lib/tags/tags-screen.svelte';

  const showingTags = $derived(page.url.searchParams.get('view') === 'tags');
  const views = [
    { tags: false, label: m.categories_title, href: '/app/ledger/categories' },
    { tags: true, label: m.recurring_tags, href: '/app/ledger/categories?view=tags' }
  ];
</script>

<div class="space-y-4">
  <nav aria-label={m.tags_categories_views()}
    class="flex w-max max-w-full flex-wrap gap-1 rounded-(--radius-panel) border border-border bg-surface p-1">
    {#each views as view (view.href)}
      <a href={view.href} aria-current={showingTags === view.tags ? 'page' : undefined}
        class="flex min-h-11 items-center rounded-(--radius-control) px-3 text-sm font-medium text-muted transition hover:text-foreground aria-[current=page]:bg-surface-strong aria-[current=page]:text-foreground">
        {view.label()}
      </a>
    {/each}
  </nav>
  {#if showingTags}
    <TagsScreen />
  {:else}
    <CategoryListScreen />
  {/if}
</div>
