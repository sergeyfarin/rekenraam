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
  import type { Attachment } from 'svelte/attachments';

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
  const onReview = $derived(entry?.route === '/app/review');
  const tabbed = $derived(section !== undefined && section !== 'home' && section !== 'settings' && section !== 'review');
  // The link's name is its visible text (label and badge) so it matches what
  // voice control users say; the status sentence rides along as a description.
  const reviewStatusID = $props.id();

  // Sticky side panels and scroll padding sit below the bar, whose height
  // changes with the section tabs and wrapping (app.css side-panel-sticky).
  const publishHeight: Attachment<HTMLElement> = (bar) => {
    const root = document.documentElement;
    const observer = new ResizeObserver(() => root.style.setProperty('--shell-bar-height', `${bar.offsetHeight}px`));
    observer.observe(bar);
    return () => {
      observer.disconnect();
      root.style.removeProperty('--shell-bar-height');
    };
  };

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

  // The current section's tab joins what it opens into: its pages row, or
  // the page itself where there is none (app.css shell-section-tab).
  const joined = $derived(tabbed ? 'pages' : 'page');
  const currentLabel = $derived(
    tabbed && section ? sectionFor(section)?.title() : section === 'settings' ? m.settings_title() : section === 'review' ? m.attention_review() : m.nav_home()
  );

  const tabClass = 'shell-section-tab px-3.5';
  // Review on its own page: a pill on phones (the menu button is the tab
  // there), the joined tab on desktop, covering the bar's border.
  const endTabClass = 'max-lg:rounded-full max-lg:hover:bg-control-hover lg:-mb-px lg:h-11 lg:self-end lg:rounded-t-[10px] lg:bg-shell-joined lg:px-3';
  const menuLinkClass =
    'flex min-h-10 items-center rounded-(--radius-control) px-3 text-sm font-medium text-muted transition hover:bg-control-hover hover:text-foreground aria-[current]:bg-surface-strong aria-[current]:text-foreground';
</script>

<a
  href="#main-content"
  class="sr-only z-50 rounded-(--radius-control) bg-surface px-3 py-2 text-sm font-semibold text-foreground focus:not-sr-only focus:fixed focus:left-4 focus:top-2"
>
  {m.nav_skip_to_content()}
</a>

<!-- Each row carries its own tint and translucency (app.css shell rows). -->
<div data-print-hide class="shell-bar sticky top-0 z-30 border-b border-border/70" {@attach publishHeight}>
  <div class="shell-sections-row" data-joined={joined}>
    <header class="mx-auto flex h-14 max-w-[100rem] items-center gap-2 px-4 sm:px-6 lg:px-8">
      <!-- Desktop: Home, then the sections as plain links to their overviews.
           The current one is a tab joined to the row below; on Home it covers
           the bar's bottom border to open into the page. -->
      <nav aria-label={m.nav_sections_label()} class="-ml-3.5 hidden min-w-0 self-end lg:block" class:-mb-px={!tabbed}>
        <ul class="flex items-end gap-0.5">
          <li><a href="/app" aria-current={onHome ? 'page' : undefined} class={tabClass}>{m.nav_home()}</a></li>
          {#each sections as item (item.id)}
            <li>
              <a href={item.overview} aria-current={sectionCurrent(item.id, item.overview)} class={tabClass}>{item.title()}</a>
            </li>
          {/each}
        </ul>
      </nav>

      <!-- Phone: the same links behind one disclosure button, drawn as the
           current section's tab. -->
      <div class="relative -ml-2.5 min-w-0 self-end lg:hidden" class:-mb-px={!tabbed} {@attach dismissible(() => menuOpen, closeMenu)}>
        <button
          bind:this={menuButton}
          type="button"
          aria-expanded={menuOpen}
          aria-controls="shell-phone-menu"
          onclick={() => (menuOpen = !menuOpen)}
          data-current
          class="shell-section-tab max-w-full gap-2 px-2.5"
        >
          {#if menuOpen}<X size={18} aria-hidden="true" />{:else}<Menu size={18} class="shrink-0" aria-hidden="true" />{/if}
          <span class="sr-only">{m.nav_menu()}:</span>
          <span class="truncate">{currentLabel}</span>
        </button>

        {#if menuOpen}
          <nav
            id="shell-phone-menu"
            aria-label={m.nav_sections_label()}
            class="absolute left-0 top-full z-40 mt-2 max-h-[calc(100dvh-4rem)] w-64 max-w-[calc(100vw-2rem)] overflow-y-auto rounded-(--radius-panel) border border-border bg-surface p-2 shadow-(--shadow-panel)"
          >
            <ul class="grid gap-1">
              <li><a href="/app" aria-current={onHome ? 'page' : undefined} class={menuLinkClass}>{m.nav_home()}</a></li>
              {#each sections as item (item.id)}
                <li>
                  <a href={item.overview} aria-current={sectionCurrent(item.id, item.overview)} class={menuLinkClass}>{item.title()}</a>
                </li>
              {/each}
            </ul>
          </nav>
        {/if}
      </div>

      <!-- Stretched so the Review link and Settings button can sit on the
           row's edge as the current tab (desktop, on their own pages). -->
      <div class="ml-auto flex min-w-0 items-center gap-1 self-stretch">
        <a href="/app/review" aria-current={onReview ? 'page' : undefined} data-current={onReview ? '' : undefined} aria-describedby={reviewStatusID}
          class="shell-end-tab inline-flex min-h-11 shrink-0 items-center gap-1.5 px-2 text-sm font-semibold text-foreground {onReview ? endTabClass : 'rounded-full hover:bg-control-hover'}">
          <ListChecks size={18} class="shrink-0 sm:hidden" aria-hidden="true" /><span class="sr-only sm:not-sr-only">{m.attention_review()}</span> <span class="rounded-full bg-nav-marker px-1.5 text-xs text-nav-marker-foreground">{reviewBadge.text}</span>
        </a>
        <span id={reviewStatusID} hidden>{reviewBadge.label}</span>
        <!-- A Search Ctrl K button opening the centred search (#490); Home
             places the same search as its command bar instead (#477). -->
        {#if !onHome}<SearchField />{/if}
        <button type="button" aria-label={m.nav_hide_amounts()} title={m.nav_hide_amounts()}
          aria-pressed={amountPrivacy.hidden} onclick={() => setAmountsHidden(!amountPrivacy.hidden)}
          class="inline-flex min-h-11 min-w-11 items-center justify-center rounded-full text-muted transition hover:bg-control-hover hover:text-foreground">
          {#if amountPrivacy.hidden}<EyeOff size={18} aria-hidden="true" />{:else}<Eye size={18} aria-hidden="true" />{/if}
        </button>
        <SettingsMenu currentRoute={entry?.route} {signOutPending} {signOutError} {onSignOut} />
      </div>
    </header>
  </div>

  {#if tabbed && section}
    <div class="shell-pages-row">
      <div class="mx-auto max-w-[100rem] px-4 sm:px-6 lg:px-8">
        <SectionTabs {section} {entry} {search} {badges} />
      </div>
    </div>
  {/if}
</div>
