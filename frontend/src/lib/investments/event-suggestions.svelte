<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import Panel from '$lib/components/panel.svelte';
  import StatePanel from '$lib/components/state-panel.svelte';
  import {
    investmentEventSuggestionsQueryOptions,
    acceptEventSuggestion,
    ignoreEventSuggestion,
    type InvestmentEventSuggestionResponse
  } from '$lib/api/investments';
  import { m } from '$lib/paraglide/messages.js';

  interface Props {
    csrfToken: string;
  }

  const { csrfToken }: Props = $props();

  const queryClient = useQueryClient();
  const suggestionsQuery = createQuery(() => investmentEventSuggestionsQueryOptions());

  const suggestions = $derived(suggestionsQuery.data?.suggestions ?? []);
  const pending = $derived(suggestions.filter((s) => s.status === 'suggested'));
  const history = $derived(suggestions.filter((s) => s.status !== 'suggested'));

  let accepting = $state<Set<number>>(new Set());
  let ignoring = $state<Set<number>>(new Set());
  let errors = $state<Map<number, string>>(new Map());

  async function accept(s: InvestmentEventSuggestionResponse) {
    accepting = new Set([...accepting, s.id]);
    errors = new Map([...errors].filter(([k]) => k !== s.id));
    try {
      await acceptEventSuggestion(s.id, csrfToken);
      await queryClient.invalidateQueries({ queryKey: ['api', 'investments', 'event-suggestions'] });
    } catch {
      errors = new Map([...errors, [s.id, m.investments_suggestion_accept_error()]]);
    } finally {
      accepting = new Set([...accepting].filter((id) => id !== s.id));
    }
  }

  async function ignore(s: InvestmentEventSuggestionResponse) {
    ignoring = new Set([...ignoring, s.id]);
    errors = new Map([...errors].filter(([k]) => k !== s.id));
    try {
      await ignoreEventSuggestion(s.id, csrfToken);
      await queryClient.invalidateQueries({ queryKey: ['api', 'investments', 'event-suggestions'] });
    } catch {
      errors = new Map([...errors, [s.id, m.investments_suggestion_ignore_error()]]);
    } finally {
      ignoring = new Set([...ignoring].filter((id) => id !== s.id));
    }
  }

  function statusLabel(status: string): string {
    switch (status) {
      case 'suggested': return m.investments_suggestion_status_suggested();
      case 'accepted': return m.investments_suggestion_status_accepted();
      case 'ignored': return m.investments_suggestion_status_ignored();
      case 'auto_posted': return m.investments_suggestion_status_auto_posted();
      case 'failed': return m.investments_suggestion_status_failed();
      case 'superseded': return m.investments_suggestion_status_superseded();
      default: return status;
    }
  }

  function statusClass(status: string): string {
    switch (status) {
      case 'suggested': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400';
      case 'accepted':
      case 'auto_posted': return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400';
      case 'ignored':
      case 'superseded': return 'bg-muted/30 text-muted';
      case 'failed': return 'bg-destructive/10 text-destructive';
      default: return 'bg-muted/30 text-muted';
    }
  }

  function confidencePct(bps: number): string {
    return `${(bps / 100).toFixed(0)}%`;
  }
</script>

<div class="space-y-6">
  {#if suggestionsQuery.isPending}
    <Panel>
      <p class="text-sm text-muted">{m.investments_suggestions_loading()}</p>
    </Panel>
  {:else if suggestionsQuery.isError}
    <Panel>
      <p class="text-sm text-destructive">{m.investments_suggestions_error()}</p>
    </Panel>
  {:else if pending.length === 0 && history.length === 0}
    <StatePanel title={m.investments_suggestions_empty()} copy="" />
  {:else}
    <!-- Pending suggestions -->
    {#if pending.length > 0}
      <div class="space-y-3">
        {#each pending as s (s.id)}
          <Panel>
            <div class="flex flex-wrap items-start justify-between gap-3">
              <div class="min-w-0 flex-1 space-y-1">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="rounded-full px-2 py-0.5 text-xs font-medium {statusClass(s.status)}">
                    {statusLabel(s.status)}
                  </span>
                  <span class="text-xs text-muted">
                    {m.investments_suggestion_confidence()}: {confidencePct(s.confidence_bps)}
                  </span>
                </div>
                <details class="mt-2">
                  <summary class="cursor-pointer text-xs text-muted hover:text-foreground">
                    {m.investments_suggestion_proposed()}
                  </summary>
                  <pre class="mt-1 overflow-x-auto rounded bg-surface-strong p-2 text-xs text-foreground">{JSON.stringify(s.proposed_transaction, null, 2)}</pre>
                </details>
                {#if errors.has(s.id)}
                  <p class="text-xs text-destructive">{errors.get(s.id)}</p>
                {/if}
              </div>
              <div class="flex shrink-0 gap-2">
                <button
                  type="button"
                  disabled={accepting.has(s.id) || ignoring.has(s.id)}
                  onclick={() => accept(s)}
                  class="rounded-(--radius-control) bg-foreground px-3 py-1.5 text-xs font-semibold text-background transition hover:opacity-90 disabled:opacity-40"
                >
                  {m.investments_suggestion_accept()}
                </button>
                <button
                  type="button"
                  disabled={accepting.has(s.id) || ignoring.has(s.id)}
                  onclick={() => ignore(s)}
                  class="rounded-(--radius-control) border border-border bg-control px-3 py-1.5 text-xs font-semibold text-foreground transition hover:bg-control-hover disabled:opacity-40"
                >
                  {m.investments_suggestion_ignore()}
                </button>
              </div>
            </div>
          </Panel>
        {/each}
      </div>
    {:else}
      <StatePanel title={m.investments_suggestions_empty()} copy="" />
    {/if}

    <!-- History -->
    {#if history.length > 0}
      <section>
        <h2 class="mb-3 text-sm font-semibold text-foreground">{m.investments_suggestions_history_title()}</h2>
        <div class="space-y-2">
          {#each history as s (s.id)}
            <Panel>
              <div class="flex flex-wrap items-center justify-between gap-2">
                <span class="rounded-full px-2 py-0.5 text-xs font-medium {statusClass(s.status)}">
                  {statusLabel(s.status)}
                </span>
                <span class="text-xs text-muted">
                  {m.investments_suggestion_confidence()}: {confidencePct(s.confidence_bps)}
                </span>
              </div>
              {#if s.failure_reason}
                <p class="mt-1 text-xs text-destructive">{s.failure_reason}</p>
              {/if}
            </Panel>
          {/each}
        </div>
      </section>
    {/if}
  {/if}
</div>
