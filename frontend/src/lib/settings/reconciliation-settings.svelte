<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { untrack } from 'svelte';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import { reconciliationPolicyQueryOptions, saveReconciliationPolicy } from '#lib/api/reconciliation.ts';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import Panel from '#lib/components/panel.svelte';
  import { parseReconciliationWindow } from '#lib/reconcile/reconciliation-age.ts';
  import { m } from '#lib/paraglide/messages.js';

  // The reconciliation out-of-date window (#482): one book setting that the
  // account list and Home's Reconciliation widget both judge with.
  const sessionQuery = createQuery(() => authSessionQueryOptions());
  const policyQuery = createQuery(() => reconciliationPolicyQueryOptions());

  let days = $state('');
  let pending = $state(false);
  let saveError = $state<unknown>(undefined);
  let saved = $state(false);
  let attempted = $state(false);

  const parsed = $derived(parseReconciliationWindow(days));
  const invalid = $derived(attempted && parsed === null);

  // Reset the field only when the stored policy changes (its save time), never
  // on a refetch that returns the same policy, so typing is not wiped.
  const policyMarker = $derived(policyQuery.data ? `${policyQuery.data.updated_at ?? 'default'}:${policyQuery.data.out_of_date_after_days}` : '');
  $effect(() => {
    if (!policyMarker) return;
    untrack(() => {
      if (pending || !policyQuery.data) return;
      days = String(policyQuery.data.out_of_date_after_days);
      attempted = false;
    });
  });

  async function csrfToken() {
    if (sessionQuery.data?.csrf_token) return sessionQuery.data.csrf_token;
    const session = await sessionQuery.refetch();
    const token = session.data?.csrf_token;
    if (!token) throw new Error('CSRF token is unavailable');
    return token;
  }

  async function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    attempted = true;
    saved = false;
    saveError = undefined;
    if (parsed === null) return;
    pending = true;
    try {
      await saveReconciliationPolicy(parsed, await csrfToken());
      await policyQuery.refetch();
      saved = true;
    } catch (error) {
      saveError = error;
    } finally {
      pending = false;
    }
  }
</script>

<Panel>
  <section aria-labelledby="reconciliation-settings-title" data-testid="reconciliation-settings">
    <h2 id="reconciliation-settings-title" class="text-lg font-semibold tracking-tight text-balance">{m.settings_reconciliation_title()}</h2>
    <p class="mt-3 max-w-3xl text-sm leading-6 text-muted">{m.settings_reconciliation_copy()}</p>
    <APIFormError error={policyQuery.error ?? saveError} />
    <form class="mt-5 grid gap-3 sm:grid-cols-[minmax(0,16rem)_auto] sm:items-end" onsubmit={handleSubmit} novalidate>
      <label class="block text-sm font-semibold text-foreground" for="reconciliation-window-days">
        {m.settings_reconciliation_days_label()}
        <input
          id="reconciliation-window-days"
          bind:value={days}
          type="text"
          inputmode="numeric"
          autocomplete="off"
          aria-describedby="reconciliation-window-hint{invalid ? ' reconciliation-window-error' : ''}"
          aria-invalid={invalid}
          class="mt-2 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground"
          disabled={pending || policyQuery.isPending}
        />
      </label>
      <button
        type="submit"
        class="min-h-11 rounded-(--radius-control) bg-accent px-4 py-2.5 text-sm font-semibold text-accent-foreground transition hover:opacity-95 disabled:cursor-not-allowed disabled:opacity-60"
        disabled={pending || policyQuery.isPending}
      >
        {pending ? m.settings_reconciliation_saving() : m.settings_reconciliation_save()}
      </button>
    </form>
    <p id="reconciliation-window-hint" class="mt-2 text-xs text-muted">{m.settings_reconciliation_days_hint()}</p>
    {#if invalid}
      <p id="reconciliation-window-error" role="alert" class="mt-2 text-sm font-semibold text-danger">{m.settings_reconciliation_invalid()}</p>
    {/if}
    {#if saved}
      <p role="status" class="mt-3 text-sm font-semibold text-positive">{m.settings_reconciliation_saved()}</p>
    {/if}
  </section>
</Panel>
