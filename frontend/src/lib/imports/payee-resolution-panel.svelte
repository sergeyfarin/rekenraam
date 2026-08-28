<script lang="ts">
  import { createQuery, useQueryClient } from '@tanstack/svelte-query';
  import APIFormError from '$lib/components/api-form-error.svelte';
  import Panel from '$lib/components/panel.svelte';
  import type { ImportResolution, ImportStagedRow } from '$lib/api/imports';
  import {
    createPayee,
    payeesQueryKey,
    payeesQueryOptions,
    type PayeeResponse
  } from '$lib/api/payees';
  import { m } from '$lib/paraglide/messages.js';
  import { unknownImportPayeeGroups } from './payee-resolution';

  let {
    rows,
    resolutions,
    csrfToken,
    onresolve
  } = $props<{
    rows: ImportStagedRow[];
    resolutions: Map<number, ImportResolution>;
    csrfToken: string;
    onresolve: (rowIDs: number[], payee: PayeeResponse) => void;
  }>();

  const queryClient = useQueryClient();
  const payeesQuery = createQuery(() => ({ ...payeesQueryOptions(), retry: false }));
  const candidateGroups = $derived(unknownImportPayeeGroups(rows, resolutions, []));
  const payees = $derived(payeesQuery.data?.payees ?? []);
  const groups = $derived(unknownImportPayeeGroups(rows, resolutions, payees));
  const showPanel = $derived(
    payeesQuery.isPending || payeesQuery.isError ? candidateGroups.length > 0 : groups.length > 0
  );

  let selectedPayeeIDs = $state<Record<string, string>>({});
  let creatingKey = $state<string | null>(null);
  let mutationError = $state<unknown>(undefined);

  function resolveWith(groupKey: string, rowIDs: number[], payee: PayeeResponse) {
    mutationError = undefined;
    onresolve(rowIDs, payee);
    selectedPayeeIDs = { ...selectedPayeeIDs, [groupKey]: '' };
  }

  function resolveSelected(groupKey: string, rowIDs: number[]) {
    const selectedID = Number(selectedPayeeIDs[groupKey]);
    const payee = payees.find((candidate) => candidate.id === selectedID);
    if (payee) resolveWith(groupKey, rowIDs, payee);
  }

  async function createAndResolve(groupKey: string, name: string, rowIDs: number[]) {
    if (!csrfToken) return;
    creatingKey = groupKey;
    mutationError = undefined;
    try {
      const created = await createPayee({ name }, csrfToken);
      resolveWith(groupKey, rowIDs, created);
      await queryClient.invalidateQueries({ queryKey: payeesQueryKey });
    } catch (error) {
      mutationError = error;
    } finally {
      creatingKey = null;
    }
  }
</script>

{#if showPanel}
  <Panel>
    <h2 class="text-sm font-semibold text-foreground">{m.import_payees_title()}</h2>
    <p class="mt-2 max-w-3xl text-sm leading-6 text-muted">{m.import_payees_copy()}</p>

    {#if payeesQuery.isPending}
      <p class="mt-4 text-sm text-muted" aria-live="polite">{m.import_payees_loading()}</p>
    {:else if payeesQuery.isError}
      <div class="mt-4 rounded-(--radius-control) border border-danger/25 bg-danger-soft p-4" role="alert">
        <p class="text-sm font-semibold text-foreground">{m.import_payees_error_title()}</p>
        <p class="mt-1 text-sm text-danger">{m.import_payees_error_copy()}</p>
        <button
          type="button"
          class="mt-3 rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-medium text-foreground transition hover:bg-control-hover"
          onclick={() => payeesQuery.refetch()}
        >
          {m.import_payees_retry()}
        </button>
      </div>
    {:else if groups.length > 0}
      <div class="mt-4 space-y-3">
        {#each groups as group, index (group.key)}
          <section class="rounded-(--radius-control) border border-border bg-surface-strong p-4" aria-labelledby={`import-payee-${index}`}>
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <h3 id={`import-payee-${index}`} class="font-semibold text-foreground">{group.name}</h3>
              <span class="text-xs text-muted">{m.import_payees_row_count({ count: group.rowIDs.length })}</span>
            </div>

            {#if group.suggestions.length > 0}
              <div class="mt-3">
                <p class="text-xs font-medium text-muted">{m.import_payees_suggestions()}</p>
                <div class="mt-2 flex flex-wrap gap-2">
                  {#each group.suggestions as suggestion (suggestion.id)}
                    <button
                      type="button"
                      class="rounded-(--radius-control) border border-border bg-control px-3 py-1.5 text-sm font-medium text-foreground transition hover:bg-control-hover"
                      onclick={() => resolveWith(group.key, group.rowIDs, suggestion)}
                    >
                      {suggestion.name}
                    </button>
                  {/each}
                </div>
              </div>
            {/if}

            <div class="mt-3 flex flex-wrap items-end gap-2">
              <div class="min-w-56 flex-1">
                <label class="text-xs font-medium text-muted" for={`import-payee-select-${index}`}>
                  {m.import_payees_existing_label()}
                </label>
                <select
                  id={`import-payee-select-${index}`}
                  class="mt-1 w-full rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-2 focus:ring-foreground"
                  value={selectedPayeeIDs[group.key] ?? ''}
                  onchange={(event) => {
                    selectedPayeeIDs = {
                      ...selectedPayeeIDs,
                      [group.key]: (event.currentTarget as HTMLSelectElement).value
                    };
                  }}
                >
                  <option value="">{m.import_payees_existing_placeholder()}</option>
                  {#each payees as payee (payee.id)}
                    <option value={payee.id}>{payee.name}</option>
                  {/each}
                </select>
              </div>
              <button
                type="button"
                class="rounded-(--radius-control) border border-border bg-control px-3 py-2 text-sm font-semibold text-foreground transition hover:bg-control-hover disabled:cursor-not-allowed disabled:opacity-60"
                disabled={!selectedPayeeIDs[group.key]}
                onclick={() => resolveSelected(group.key, group.rowIDs)}
              >
                {m.import_payees_link()}
              </button>
              <button
                type="button"
                class="rounded-(--radius-control) bg-foreground px-3 py-2 text-sm font-semibold text-background transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
                disabled={creatingKey !== null || !csrfToken}
                onclick={() => createAndResolve(group.key, group.name, group.rowIDs)}
              >
                {creatingKey === group.key ? m.import_payees_creating() : m.import_payees_create({ name: group.name })}
              </button>
            </div>
          </section>
        {/each}
      </div>
    {/if}

    <div class="mt-4">
      <APIFormError error={mutationError} id="import-payee-error" />
    </div>
  </Panel>
{/if}
