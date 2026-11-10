<script lang="ts">
  import { autofocus } from '#lib/components/autofocus.ts';
  import ArrowUpDown from '@lucide/svelte/icons/arrow-up-down';
  import { m } from '#lib/paraglide/messages.js';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import type { ReorderMode } from './reorder-mode.svelte.ts';

  // The reorder switch of a list screen (#185). Off, it is one quiet button.
  // On, it explains what moves, offers "Only in this account" where a
  // statement can list a day differently, and asks before a move changes a
  // reconciled balance.
  let { mode, offerOnlyThisAccount = false, disabled = false }: {
    mode: ReorderMode;
    offerOnlyThisAccount?: boolean;
    disabled?: boolean;
  } = $props();

  const switchID = $props.id();
</script>

{#if !mode.active}
  <button
    type="button"
    class="inline-flex items-center gap-2 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover disabled:opacity-60"
    {disabled}
    onclick={() => mode.start()}
  >
    <ArrowUpDown size={14} aria-hidden="true" />
    {m.transactions_reorder_start()}
  </button>
{:else}
  <div class="w-full space-y-2 rounded-(--radius-control) border border-accent/40 bg-surface px-3 py-2" role="group" aria-label={m.transactions_reorder_start()}>
    <div class="flex flex-wrap items-center justify-between gap-2">
      <p class="text-sm text-foreground" aria-live="polite">
        {mode.onlyThisAccount ? m.transactions_reorder_help_account() : m.transactions_reorder_help()}
      </p>
      <button
        type="button"
        class="rounded-(--radius-control) bg-accent px-3 py-2 text-sm font-semibold text-accent-foreground transition hover:opacity-90"
        onclick={() => mode.finish()}
      >
        {m.transactions_reorder_done()}
      </button>
    </div>
    {#if offerOnlyThisAccount}
      <label for={switchID} class="flex items-center gap-2 text-sm text-foreground">
        <input id={switchID} type="checkbox" bind:checked={mode.onlyThisAccount} class="h-4 w-4" />
        {m.transactions_reorder_only_account()}
      </label>
    {/if}
    {#if mode.dayMove}
      <div class="space-y-2 rounded-(--radius-control) border border-accent/40 bg-surface px-3 py-2" role="alert">
        <p class="text-sm text-foreground">{m.transactions_reorder_day_confirm({ date: mode.dayMove.date })}</p>
        <div class="flex flex-wrap gap-2">
          <button
            type="button"
            class="rounded-(--radius-control) bg-accent px-3 py-2 text-sm font-semibold text-accent-foreground transition hover:opacity-90"
            disabled={mode.pending}
            onclick={() => mode.confirmDayMove()}
            {@attach autofocus}
          >
            {m.transactions_reorder_day_confirm_button({ date: mode.dayMove.date })}
          </button>
          <button
            type="button"
            class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground"
            onclick={() => mode.cancelDayMove()}
          >
            {m.investments_form_cancel()}
          </button>
        </div>
      </div>
    {/if}
    {#if mode.awaitingOverride}
      <div class="space-y-2 rounded-(--radius-control) border border-warning/50 bg-warning-soft px-3 py-2" role="alert">
        <p class="text-sm text-foreground">{m.transactions_reorder_override()}</p>
        <div class="flex flex-wrap gap-2">
          <button
            type="button"
            class="rounded-(--radius-control) border border-warning/50 bg-control px-3 py-2 text-sm font-semibold text-foreground"
            disabled={mode.pending}
            onclick={() => mode.confirmOverride()}
          >
            {m.transactions_reorder_override_confirm()}
          </button>
          <button
            type="button"
            class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground"
            onclick={() => mode.cancelOverride()}
          >
            {m.investments_form_cancel()}
          </button>
        </div>
      </div>
    {/if}
    <APIFormError error={mode.error} />
  </div>
{/if}
