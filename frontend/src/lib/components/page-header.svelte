<script lang="ts">
  import type { Snippet } from 'svelte';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import type { Crumb } from '#lib/navigation/routes.ts';
  import { m } from '#lib/paraglide/messages.js';

  // The page header (#459): a breadcrumb on detail pages, the title (or, in
  // `heading`, the object switcher of #442), a scope line where the page has
  // one, and the page's main action on the right, so the action is a button a
  // phone can tap rather than a keyboard shortcut.
  let {
    breadcrumbs = [],
    title,
    heading,
    scope,
    copy,
    actions
  }: {
    breadcrumbs?: Crumb[];
    title: string;
    /** Replaces the title text inside the h1, such as an object switcher. */
    heading?: Snippet;
    /** What the page shows: its period, account or filters in one line. */
    scope?: Snippet;
    copy?: string;
    actions?: Snippet;
  } = $props();
</script>

<!-- No horizontal padding of its own: the shell's content column sets the one
     left edge the header, tab row and page share (#436). -->
<header class="pb-4">
  {#if breadcrumbs.length > 0}
    <nav aria-label={m.nav_breadcrumb()} class="mb-1">
      <ol class="flex flex-wrap items-center gap-1 text-sm text-muted">
        {#each breadcrumbs as crumb, index (crumb.href)}
          <li class="flex items-center gap-1">
            {#if index > 0}<ChevronRight size={14} aria-hidden="true" />{/if}
            <a href={crumb.href} class="inline-flex min-h-6 items-center font-semibold underline-offset-2 transition hover:text-foreground hover:underline">{crumb.label}</a>
          </li>
        {/each}
      </ol>
    </nav>
  {/if}
  <div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-3">
    <div class="min-w-0 flex-1 basis-48">
      <h1 class="truncate text-2xl font-semibold tracking-tight text-foreground sm:text-3xl">
        {#if heading}{@render heading()}{:else}{title}{/if}
      </h1>
      {#if scope}
        <p class="mt-1 text-sm text-muted">{@render scope()}</p>
      {/if}
      {#if copy}
        <p class="mt-2 max-w-4xl text-sm leading-6 text-muted">{copy}</p>
      {/if}
    </div>

    {#if actions}
      <div class="flex max-w-full shrink-0 flex-wrap items-center gap-2">
        {@render actions()}
      </div>
    {/if}
  </div>
</header>
