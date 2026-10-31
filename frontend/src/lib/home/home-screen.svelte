<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import { homeQueryOptions } from '#lib/api/home.ts';
  import { useAttention } from '#lib/attention/context.ts';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { useOwnerToday } from '#lib/dates/owner-date-context.ts';
  import { dismissible } from '#lib/navigation/disclosure.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';
  import SearchField from '#lib/search/search-field.svelte';
  import HomeShortcuts from './home-shortcuts.svelte';
  import HomeWidgets from './home-widgets.svelte';
  import { briefingHeadline, briefingStatus, headlineOptions, shownHeadlineID, type HeadlineChoice } from './briefing.ts';

  const ownerToday = useOwnerToday();
  const attention = useAttention();
  const homeQuery = createQuery(() => homeQueryOptions(ownerToday()));
  const locale = $derived(getLocale());

  // Not remembered: choosing and keeping the headline is personalization (#438).
  let choice = $state<HeadlineChoice>('auto');
  let menuOpen = $state(false);
  let menuButton = $state<HTMLButtonElement>();

  const home = $derived(homeQuery.data);
  const shown = $derived(home ? shownHeadlineID(home, choice) : null);
  const headline = $derived(home && shown ? briefingHeadline(home, shown, locale) : null);
  const options = $derived(home ? headlineOptions(home, locale) : []);
  const status = $derived(briefingStatus(home, shown, choice, attention(), locale));

  function closeMenu(restoreFocus: boolean) {
    menuOpen = false;
    if (restoreFocus) menuButton?.focus();
  }

  function choose(id: HeadlineChoice) {
    choice = id;
    closeMenu(true);
  }
</script>

<!-- Home shows no title; the briefing's first line is its visible heading in
     every state, so the first thing read after "Home" is what the page says. -->
<h1 class="sr-only">{m.nav_home()}</h1>

<section class="mx-auto max-w-[45rem] py-4 text-center sm:py-8">
  {#if homeQuery.isPending}
    <h2 role="status" class="text-lg font-normal text-muted">{m.briefing_loading()}</h2>
  {:else if homeQuery.isError}
    <div role="alert">
      <h2 class="text-lg font-semibold">{m.briefing_load_error()}</h2>
      <p class="mt-2 text-sm text-muted">{m.briefing_check_failed()}</p>
    </div>
    <div class="mt-4 space-y-3">
      <APIFormError error={homeQuery.error} />
      <button type="button" onclick={() => homeQuery.refetch()} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-4 text-sm font-semibold">{m.attention_retry()}</button>
    </div>
  {:else if home?.empty_book}
    <div class="rounded-(--radius-panel) border border-border bg-surface p-6 text-left">
      <h2 class="text-lg font-semibold">{m.briefing_start_title()}</h2>
      <p class="mt-2 text-sm leading-6 text-muted">{m.briefing_start_copy()}</p>
      <a href="/app/ledger/accounts" class="mt-4 inline-flex min-h-11 items-center rounded-(--radius-control) bg-accent px-4 text-sm font-semibold text-accent-foreground">{m.briefing_start_action()}</a>
    </div>
  {:else if home && headline}
    <div class="relative" {@attach dismissible(() => menuOpen, closeMenu)}>
      <h2 class="text-lg font-normal leading-8 text-foreground sm:text-xl" data-testid="home-headline">
        <button
          bind:this={menuButton}
          type="button"
          aria-expanded={menuOpen}
          aria-controls="home-headline-menu"
          aria-label={`${headline.label}, ${m.briefing_headline_menu()}`}
          onclick={() => (menuOpen = !menuOpen)}
          class="inline-flex min-h-11 items-center gap-1 rounded-(--radius-control) px-1 align-middle text-foreground hover:bg-control-hover"
        >
          {headline.label}<ChevronDown size={16} aria-hidden="true" class="text-muted" />
        </button>
        {#if headline.amount === null}
          <span class="text-muted">{m.briefing_headline_unavailable({ label: headline.label })}</span>
        {:else}
          <span class="font-medium tabular-nums">{headline.amount}</span>
          {#if headline.change}
            <span aria-hidden="true" class="whitespace-nowrap tabular-nums {headline.change.sign > 0 ? 'text-positive' : headline.change.sign < 0 ? 'text-danger' : 'text-muted'}">{headline.change.text}</span>
            <span class="sr-only">{headline.change.label}</span>
          {/if}
        {/if}
      </h2>
      {#if menuOpen}
        <div id="home-headline-menu" role="group" aria-label={m.briefing_headline_menu()} class="absolute left-1/2 top-full z-30 mt-1 w-64 max-w-[calc(100vw-2rem)] -translate-x-1/2 rounded-(--radius-panel) border border-border bg-surface p-1 text-left text-sm shadow-(--shadow-panel)">
          {#each options as option (option.id)}
            <button
              type="button"
              aria-pressed={shown === option.id}
              onclick={() => choose(option.id)}
              class="flex min-h-11 w-full flex-col justify-center rounded-(--radius-control) px-3 py-1 text-left hover:bg-control-hover aria-pressed:bg-surface-strong"
            >
              <span class="font-medium">{option.label}</span>
              <span class="text-xs text-muted">{option.scope}</span>
            </button>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  {#if home && !home.empty_book && !homeQuery.isError}
    <!-- Without a headline (cash lacks a rate too) the status line is the heading. -->
    <svelte:element this={headline ? 'p' : 'h2'} class="mt-1 text-lg font-normal leading-8 text-muted sm:text-xl" data-testid="home-status">
      <a href={status.lead.href} class="underline-offset-4 hover:underline {status.tone === 'failed' ? 'text-danger' : status.tone === 'attention' ? 'text-foreground' : ''}">{status.lead.text}{#if !status.more}&nbsp;→{/if}</a>{#if status.more}<span aria-hidden="true"> · </span><a href={status.more.href} aria-label={status.more.label} class="whitespace-nowrap underline-offset-4 hover:underline">{status.more.text}&nbsp;→</a>{/if}
    </svelte:element>
  {/if}
</section>

<!-- The command bar and shortcuts work whatever the briefing's state: search
     needs no Home data, and an empty book still imports or adds (#477). -->
<div class="mx-auto max-w-[45rem]">
  <SearchField variant="home" />
</div>
<HomeShortcuts />
<HomeWidgets home={homeQuery.data} failed={homeQuery.isError && !homeQuery.data} onRetry={() => homeQuery.refetch()} />
