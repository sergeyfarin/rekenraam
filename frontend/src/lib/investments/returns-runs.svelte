<script lang="ts">
  import { createInfiniteQuery, createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query';
  import Panel from '#lib/components/panel.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import type { CurrencyResponse } from '#lib/api/currencies.ts';
  import {
    getReturnsRun, getReturnsRuns, returnsRunQueryKey, returnsRunsQueryKey, saveReturnsRun, type ReturnsRunRequest
  } from '#lib/api/investments.ts';
  import { m } from '#lib/paraglide/messages.js';
  import ReturnsResult from './returns-result.svelte';

  // Saved return runs (R13 slice 6, #119): save the applied selection as an
  // immutable returns_v1 document, list them and reopen one. A reopened run
  // reads only its saved bytes; it never queries the live read.
  let {
    request, csrfToken, currencies, locale, accountName, currencyOrCommodity, linkedRunID = null
  }: {
    /** The applied selection, or null when there is no live result to save. */
    request: ReturnsRunRequest | null;
    csrfToken: string;
    currencies: CurrencyResponse[];
    locale: string;
    accountName: (id: number) => string;
    currencyOrCommodity: (id: number) => string;
    /** A run a link names (`?run_id=`, as Home's Returns and Allocation link it): opened and focused. */
    linkedRunID?: number | null;
  } = $props();

  let selectedID = $state(0);
  let revealed = false;
  $effect.pre(() => {
    if (linkedRunID !== null) {
      selectedID = linkedRunID;
      revealed = false;
    }
  });

  /** Moves focus, and so the view, to the run the link opened, once. */
  function reveal(node: HTMLElement) {
    if (!revealed && linkedRunID !== null && run?.id === linkedRunID) {
      revealed = true;
      node.focus();
    }
  }
  const client = useQueryClient();
  const instants = $derived(new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }));
  const dates = $derived(new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }));

  const save = createMutation(() => ({
    mutationFn: (input: ReturnsRunRequest) => saveReturnsRun(input, csrfToken),
    onSuccess: async (run) => {
      client.setQueryData(returnsRunQueryKey(run.id), run);
      selectedID = run.id;
      await client.invalidateQueries({ queryKey: returnsRunsQueryKey });
    }
  }));
  const list = createInfiniteQuery(() => ({
    queryKey: returnsRunsQueryKey, initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => getReturnsRuns(pageParam), getNextPageParam: (last) => last.next_cursor ?? undefined
  }));
  const runs = $derived(list.data?.pages.flatMap((page) => page.runs) ?? []);
  // A saved document never changes: cached for good once read.
  const query = createQuery(() => ({
    queryKey: returnsRunQueryKey(selectedID), queryFn: () => getReturnsRun(selectedID), enabled: selectedID > 0, staleTime: Infinity
  }));
  const run = $derived(query.data);

  function day(date: string): string {
    return dates.format(new Date(`${date}T00:00:00Z`));
  }
</script>

<Panel>
  <section class="min-w-0 space-y-3" aria-labelledby="returns-runs-title" data-testid="returns-runs">
    <h2 id="returns-runs-title" class="text-sm font-semibold text-foreground">{m.returns_runs_title()}</h2>
    <p class="text-sm text-muted">{m.returns_runs_note()}</p>

    <div class="space-y-2" data-testid="save-returns-run">
      <button type="button" disabled={request === null || save.isPending || !csrfToken} onclick={() => { if (request) save.mutate({ ...request }); }}
        class="min-h-11 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-medium text-foreground hover:bg-control-hover disabled:opacity-50">
        {save.isPending ? m.returns_runs_saving() : m.returns_runs_save()}
      </button>
      <p class="text-xs text-muted">{request === null ? m.returns_runs_save_unavailable() : m.returns_runs_save_note()}</p>
      {#if save.isError}
        <div role="alert" class="space-y-1 text-sm text-danger">
          <p>{m.returns_runs_save_error()}</p>
          <APIFormError error={save.error} />
        </div>
      {/if}
      {#if save.isSuccess}<p role="status" class="text-sm text-foreground">{m.returns_runs_saved()}</p>{/if}
    </div>

    {#if list.isPending}<p role="status" class="text-sm text-muted">{m.returns_runs_loading()}</p>
    {:else if list.isError}
      <p role="alert" class="text-sm text-danger">{m.returns_runs_load_error()}</p>
      <button type="button" class="min-h-11 text-sm text-foreground underline" onclick={() => list.refetch()}>{m.install_gate_retry()}</button>
    {:else if runs.length === 0}<p class="text-sm text-muted">{m.returns_runs_empty()}</p>
    {:else}
      <ul class="grid gap-2 sm:grid-cols-2">
        {#each runs as item (item.id)}
          <li>
            <button type="button" aria-pressed={selectedID === item.id} onclick={() => { selectedID = item.id; }}
              class="min-h-11 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-left text-sm text-foreground hover:bg-control-hover">
              {m.returns_runs_item({ id: String(item.id), date: instants.format(new Date(item.captured_at)) })}
              <span class="block text-xs text-muted">{m.returns_runs_item_period({
                from: day(item.inputs.descriptor.from), to: day(item.inputs.descriptor.to),
                currency: currencies.find((currency) => currency.id === item.inputs.request.reporting_currency_id)?.code ?? `#${item.inputs.request.reporting_currency_id}`
              })}</span>
              {#if item.inputs.request.instruments}<span class="block text-xs text-muted">{m.returns_runs_item_instruments()}</span>{/if}
            </button>
          </li>
        {/each}
      </ul>
      {#if list.hasNextPage}
        <button type="button" disabled={list.isFetchingNextPage} onclick={() => list.fetchNextPage()} class="min-h-11 text-sm text-foreground underline">{m.returns_runs_more()}</button>
      {/if}
    {/if}

    {#if selectedID > 0}
      <div class="min-w-0 space-y-3 border-t border-border pt-3" data-testid="saved-returns-run">
        {#if query.isPending}<p role="status" class="text-sm text-muted">{m.returns_runs_loading()}</p>
        {:else if query.isError}
          <div role="alert" class="space-y-1 text-sm text-danger">
            <p>{m.returns_runs_open_error()}</p>
            <APIFormError error={query.error} />
          </div>
          <button type="button" class="min-h-11 text-sm text-foreground underline" onclick={() => query.refetch()}>{m.install_gate_retry()}</button>
        {:else if run}
          <h3 tabindex="-1" {@attach reveal} class="text-sm font-semibold text-foreground">{m.returns_runs_selected({ id: String(run.id), date: instants.format(new Date(run.captured_at)) })}</h3>
          <p class="text-sm text-muted">{m.returns_runs_frozen_note()}</p>
          <div class="flex flex-wrap gap-2">
            <a href={`/api/v1/investments/returns/runs/${run.id}?download=true`}
              class="inline-flex min-h-11 items-center rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground hover:bg-control-hover">{m.returns_runs_download()}</a>
            <button type="button" class="min-h-11 text-sm text-foreground underline" onclick={() => { selectedID = 0; }}>{m.returns_runs_close()}</button>
          </div>
          <details class="text-xs text-muted">
            <summary class="min-h-11 cursor-pointer py-3">{m.returns_runs_fingerprints()}</summary>
            <dl class="space-y-2 break-all">
              <dt>{m.returns_runs_inputs()}</dt><dd class="font-mono">{run.inputs_sha256}</dd>
              <dt>{m.returns_runs_output()}</dt><dd class="font-mono">{run.output_sha256}</dd>
            </dl>
          </details>
          {#if run.output.resolved_account_ids.length === 0}
            <p class="text-sm text-muted">{m.returns_empty_copy()}</p>
          {:else}
            <ReturnsResult result={run.output} reportingCurrencyID={run.inputs.request.reporting_currency_id} {currencies} {locale}
              {accountName} {currencyOrCommodity} saved />
          {/if}
        {/if}
      </div>
    {/if}
  </section>
</Panel>
