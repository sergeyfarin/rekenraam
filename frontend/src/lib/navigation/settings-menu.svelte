<script lang="ts">
  import { afterNavigate } from '$app/navigation';
  import LogOut from '@lucide/svelte/icons/log-out';
  import Settings from '@lucide/svelte/icons/settings';
  import { m } from '#lib/paraglide/messages.js';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { dismissible } from './disclosure';
  import { sectionTabs } from './routes';
  import ThemeChoice from './theme-choice.svelte';

  let {
    currentRoute,
    signOutPending,
    signOutError,
    onSignOut
  }: {
    currentRoute: string | undefined;
    signOutPending: boolean;
    signOutError: unknown;
    onSignOut: () => void;
  } = $props();

  let open = $state(false);
  let button = $state<HTMLButtonElement>();
  const pages = sectionTabs('settings');
  const onSettings = $derived(currentRoute?.startsWith('/app/settings') ?? false);

  function close(restoreFocus: boolean) {
    open = false;
    if (restoreFocus) button?.focus();
  }

  afterNavigate(() => {
    open = false;
  });
</script>

<!-- Stretched to the row so the button can sit on its edge as the current
     tab on desktop settings pages (app-header.svelte). -->
<div class="relative flex items-center self-stretch" {@attach dismissible(() => open, close)}>
  <button
    bind:this={button}
    type="button"
    aria-expanded={open}
    aria-controls="shell-settings-menu"
    aria-label={m.settings_title()}
    title={m.settings_title()}
    onclick={() => (open = !open)}
    data-current={onSettings ? '' : undefined}
    class="shell-end-tab inline-flex size-10 items-center justify-center rounded-(--radius-control) transition {onSettings
      ? 'text-foreground max-lg:hover:bg-control-hover max-lg:aria-expanded:bg-surface-strong lg:-mb-px lg:size-11 lg:self-end lg:rounded-b-none lg:rounded-t-[10px] lg:bg-shell-joined'
      : 'text-muted hover:bg-control-hover hover:text-foreground aria-expanded:bg-surface-strong aria-expanded:text-foreground'}"
  >
    <Settings size={18} aria-hidden="true" />
  </button>

  {#if open}
    <div
      id="shell-settings-menu"
      class="absolute right-0 top-full z-40 mt-2 max-h-[calc(100dvh-4rem)] w-72 max-w-[calc(100vw-2rem)] overflow-y-auto rounded-(--radius-panel) border border-border bg-surface p-2 shadow-(--shadow-panel)"
    >
      <nav aria-label={m.settings_title()}>
        <ul>
          {#each pages as entry (entry.route)}
            <li>
              <a
                href={entry.route}
                aria-current={currentRoute === entry.route ? 'page' : undefined}
                class="flex min-h-10 items-center rounded-(--radius-control) px-3 text-sm font-medium text-foreground transition hover:bg-control-hover aria-[current=page]:bg-surface-strong"
              >
                {entry.title()}
              </a>
            </li>
          {/each}
        </ul>
      </nav>

      <div class="mt-2 border-t border-border px-1 pt-3">
        <p class="mb-2 px-2 text-xs font-semibold text-muted">{m.theme_label()}</p>
        <ThemeChoice />
      </div>

      <div class="mt-3 border-t border-border pt-2">
        <APIFormError error={signOutError} id="shell-sign-out-error" />
        <button
          type="button"
          onclick={onSignOut}
          disabled={signOutPending}
          class="flex min-h-10 w-full items-center gap-2 rounded-(--radius-control) px-3 text-sm font-medium text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60"
        >
          <LogOut size={16} aria-hidden="true" />
          {signOutPending ? m.nav_signing_out() : m.nav_sign_out()}
        </button>
      </div>
    </div>
  {/if}
</div>
