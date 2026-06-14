<script lang="ts">
  import { page } from '$app/stores';
  import type { Snippet } from 'svelte';
  import Coins from '@lucide/svelte/icons/coins';
  import Palette from '@lucide/svelte/icons/palette';
  import Settings2 from '@lucide/svelte/icons/settings-2';
  import { m } from '$lib/paraglide/messages.js';

  let { children } = $props<{ children?: Snippet }>();

  const pathname = $derived($page.url.pathname);

  const sections = $derived([
    {
      href: '/app/settings',
      label: m.settings_nav_overview(),
      icon: Settings2,
      active: pathname === '/app/settings'
    },
    {
      href: '/app/settings/appearance',
      label: m.settings_nav_appearance(),
      icon: Palette,
      active: pathname.startsWith('/app/settings/appearance')
    },
    {
      href: '/app/settings/currencies',
      label: m.settings_nav_currencies(),
      icon: Coins,
      active: pathname.startsWith('/app/settings/currencies')
    }
  ]);
</script>

<div class="grid gap-4 lg:grid-cols-[15rem_minmax(0,1fr)]">
  <aside class="min-w-0">
    <nav
      class="flex gap-2 overflow-x-auto rounded-(--radius-panel) border border-border bg-sidebar p-2 lg:block lg:space-y-1 lg:overflow-visible"
      aria-label={m.settings_nav_label()}
    >
      {#each sections as section}
        {@const Icon = section.icon}
        <a
          href={section.href}
          aria-current={section.active ? 'page' : undefined}
          class:bg-selected={section.active}
          class:text-selected-foreground={section.active}
          class:bg-transparent={!section.active}
          class:text-foreground={!section.active}
          class="flex min-w-fit items-center gap-2 rounded-(--radius-control) px-3 py-2 text-sm font-semibold transition hover:bg-control-hover"
        >
          <Icon size={16} aria-hidden="true" />
          {section.label}
        </a>
      {/each}
    </nav>
  </aside>

  <div class="min-w-0">
    {#if children}
      {@render children()}
    {/if}
  </div>
</div>
