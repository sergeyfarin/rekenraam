<script lang="ts">
  import type { AttentionBadge } from '#lib/attention/model.ts';
  import { afterNavigate } from '$app/navigation';
  import ListChecks from '@lucide/svelte/icons/list-checks';
  import Eye from '@lucide/svelte/icons/eye';
  import EyeOff from '@lucide/svelte/icons/eye-off';
  import { amountPrivacy, setAmountsHidden } from '#lib/money/privacy.svelte.ts';
  import Menu from '@lucide/svelte/icons/menu';
  import X from '@lucide/svelte/icons/x';
  import { m } from '#lib/paraglide/messages.js';
  import { dismissible } from './disclosure';
  import { sectionFor, sections, type RouteEntry } from './routes';
  import SectionTabs from './section-tabs.svelte';
  import SearchField from '#lib/search/search-field.svelte';
  import SettingsMenu from './settings-menu.svelte';

  let {
    entry,
    search,
    badges,
    reviewBadge,
    signOutPending,
    signOutError,
    onSignOut
  }: {
    entry: RouteEntry | undefined;
    search: string;
    badges: Map<string, AttentionBadge>;
    reviewBadge: AttentionBadge;
    signOutPending: boolean;
    signOutError: unknown;
    onSignOut: () => void;
  } = $props();

  const section = $derived(entry?.section);
  const onHome = $derived(entry?.route === '/app');
  const tabbed = $derived(section !== undefined && section !== 'home' && section !== 'settings' && section !== 'review');
  // The link's name is its visible text (label and badge) so it matches what
  // voice control users say; the status sentence rides along as a description.
  const reviewStatusID = $props.id();

  let menuOpen = $state(false);
  let menuButton = $state<HTMLButtonElement>();

  function closeMenu(restoreFocus: boolean) {
    menuOpen = false;
    if (restoreFocus) menuButton?.focus();
  }

  afterNavigate(() => {
    menuOpen = false;
  });

  // A section link is the current page on its overview and marks the section
  // (not the page) everywhere else in it.
  function sectionCurrent(id: string, overview: string): 'page' | 'true' | undefined {
    if (entry?.route === overview) return 'page';
    return section === id ? 'true' : undefined;
  }

  const linkClass =
    'flex min-h-10 items-center rounded-full px-3 text-sm font-medium text-muted transition hover:text-foreground aria-[current]:bg-surface-strong aria-[current]:text-foreground';
</script>

<a
  href="#main-content"
  class="sr-only z-50 rounded-(--radius-control) bg-surface px-3 py-2 text-sm font-semibold text-foreground focus:not-sr-only focus:fixed focus:left-4 focus:top-2"
>
  {m.nav_skip_to_content()}
</a>

<div data-print-hide class="shell-bar sticky top-0 z-30 border-b border-border/70">
  <div class="mx-auto max-w-[100rem] px-4 sm:px-6 lg:px-8">
    <header class="flex h-14 items-center gap-2">
      <!-- Desktop: Home, then the sections as plain links to their overviews. -->
      <nav aria-label={m.nav_sections_label()} class="-ml-3 hidden min-w-0 lg:block">
        <ul class="flex items-center gap-1">
          <li><a href="/app" aria-current={onHome ? 'page' : undefined} class={linkClass}>{m.nav_home()}</a></li>
          {#each sections as item (item.id)}
            <li>
              <a href={item.overview} aria-current={sectionCurrent(item.id, item.overview)} class={linkClass}>{item.title()}</a>
            </li>
          {/each}
        </ul>
      </nav>

      <!-- Phone: the same links behind one disclosure button. -->
      <div class="relative -ml-2 min-w-0 lg:hidden" {@attach dismissible(() => menuOpen, closeMenu)}>
        <button
          bind:this={menuButton}
          type="button"
          aria-expanded={menuOpen}
          aria-controls="shell-phone-menu"
          onclick={() => (menuOpen = !menuOpen)}
          class="inline-flex min-h-10 max-w-full items-center gap-2 rounded-(--radius-control) px-2 text-sm font-semibold text-foreground transition hover:bg-control-hover"
        >
          {#if menuOpen}<X size={18} aria-hidden="true" />{:else}<Menu size={18} class="shrink-0" aria-hidden="true" />{/if}
          <span class="sr-only">{m.nav_menu()}:</span>
          <span class="truncate">{section && section !== 'home' && section !== 'settings' && section !== 'review' ? sectionFor(section)?.title() : section === 'settings' ? m.settings_title() : section === 'review' ? m.attention_review() : m.nav_home()}</span>
        </button>

        {#if menuOpen}
          <nav
            id="shell-phone-menu"
            aria-label={m.nav_sections_label()}
            class="absolute left-0 top-full z-40 mt-2 max-h-[calc(100dvh-4rem)] w-64 max-w-[calc(100vw-2rem)] overflow-y-auto rounded-(--radius-panel) border border-border bg-surface p-2 shadow-(--shadow-panel)"
          >
            <ul class="grid gap-1">
              <li><a href="/app" aria-current={onHome ? 'page' : undefined} class={linkClass}>{m.nav_home()}</a></li>
              {#each sections as item (item.id)}
                <li>
                  <a href={item.overview} aria-current={sectionCurrent(item.id, item.overview)} class={linkClass}>{item.title()}</a>
                </li>
              {/each}
            </ul>
          </nav>
        {/if}
      </div>

      <div class="ml-auto flex min-w-0 items-center gap-1">
        <a href="/app/review" aria-current={entry?.route === '/app/review' ? 'page' : undefined} aria-describedby={reviewStatusID} class="inline-flex min-h-11 shrink-0 items-center gap-1.5 rounded-full px-2 text-sm font-semibold text-foreground hover:bg-control-hover">
          <ListChecks size={18} class="shrink-0 sm:hidden" aria-hidden="true" /><span class="sr-only sm:not-sr-only">{m.attention_review()}</span> <span class="rounded-full bg-foreground px-1.5 text-xs text-background">{reviewBadge.text}</span>
        </a>
        <span id={reviewStatusID} hidden>{reviewBadge.label}</span>
        <!-- Home places the same search large and centred instead (#477). -->
        {#if !onHome}<div class="min-w-0 md:w-48 xl:w-60"><SearchField /></div>{/if}
        <button type="button" aria-label={m.nav_hide_amounts()} title={m.nav_hide_amounts()}
          aria-pressed={amountPrivacy.hidden} onclick={() => setAmountsHidden(!amountPrivacy.hidden)}
          class="inline-flex min-h-11 min-w-11 items-center justify-center rounded-full text-muted transition hover:bg-control-hover hover:text-foreground">
          {#if amountPrivacy.hidden}<EyeOff size={18} aria-hidden="true" />{:else}<Eye size={18} aria-hidden="true" />{/if}
        </button>
        <SettingsMenu currentRoute={entry?.route} {signOutPending} {signOutError} {onSignOut} />
      </div>
    </header>

    {#if tabbed && section}
      <SectionTabs {section} {entry} {search} {badges} />
    {/if}
  </div>
</div>
