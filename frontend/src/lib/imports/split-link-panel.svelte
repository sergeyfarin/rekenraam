<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import { parseISO } from 'date-fns';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import type { AccountResponse } from '#lib/api/accounts.ts';
  import { importSplitCandidates, importSplitCandidatesQueryKey, linkImportSplit } from '#lib/api/imports.ts';
  import { m } from '#lib/paraglide/messages.js';
  import { getLocale } from '#lib/paraglide/runtime.js';

  let { batchId, rowId, csrfToken, accounts, onLinked }: {
    batchId: number;
    rowId: number;
    csrfToken: string;
    accounts: AccountResponse[];
    onLinked: () => void | Promise<void>;
  } = $props();

  const candidatesQuery = createQuery(() => ({
    queryKey: [...importSplitCandidatesQueryKey, batchId, rowId],
    queryFn: () => importSplitCandidates(batchId, rowId)
  }));
  let selected = $state('');
  let pending = $state(false);
  let linkError = $state<unknown>(undefined);

  const dateFormatter = $derived(new Intl.DateTimeFormat(getLocale(), { year: 'numeric', month: 'short', day: 'numeric' }));
  const candidates = $derived(candidatesQuery.data?.candidates ?? []);

  async function link(event: SubmitEvent) {
    event.preventDefault();
    if (!selected || pending || !csrfToken) return;
    pending = true;
    linkError = undefined;
    try {
      await linkImportSplit(batchId, rowId, Number(selected), csrfToken);
      await onLinked();
    } catch (error) {
      linkError = error;
    } finally {
      pending = false;
    }
  }
</script>

<form class="max-w-xl space-y-3" onsubmit={link} aria-busy={pending}>
  <p class="text-sm text-muted">{m.import_preview_split_link_scope()}</p>
  {#if candidatesQuery.isPending}
    <p class="text-sm text-muted" role="status">{m.import_preview_split_link_loading()}</p>
  {:else if candidatesQuery.isError}
    <APIFormError error={candidatesQuery.error} id={`split-link-load-error-${rowId}`} />
  {:else if candidates.length === 0}
    <p class="text-sm text-foreground" role="status">
      {m.import_preview_split_link_empty()}
      <a href="/app/investments" class="font-semibold underline underline-offset-2">{m.import_preview_split_link_record()}</a>
    </p>
  {:else}
    <fieldset class="space-y-2">
      <legend class="text-sm font-medium text-foreground">{m.import_preview_split_link_choose()}</legend>
      {#each candidates as candidate (candidate.operation_id)}
        <label class="flex items-start gap-2 text-sm text-foreground" class:opacity-60={candidate.linked}>
          <input type="radio" name={`split-link-${rowId}`} value={String(candidate.operation_id)}
            bind:group={selected} disabled={candidate.linked || pending} class="mt-1 h-4 w-4" />
          <span>
            {m.import_preview_split_link_option({
              date: dateFormatter.format(parseISO(candidate.effective_on)),
              ratio: `${candidate.ratio_numerator}:${candidate.ratio_denominator}`,
              account: accounts.find((account) => account.id === candidate.holding_account_id)?.name ?? `#${candidate.holding_account_id}`
            })}
            {#if candidate.linked}<span class="block text-xs text-muted">{m.import_preview_split_link_already()}</span>{/if}
          </span>
        </label>
      {/each}
    </fieldset>
    <APIFormError error={linkError} id={`split-link-error-${rowId}`} />
    <button type="submit" disabled={!selected || pending || !csrfToken}
      class="rounded-(--radius-control) bg-foreground px-4 py-2 text-sm font-semibold text-background hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60">
      {pending ? m.import_preview_split_link_pending() : m.import_preview_split_link_submit()}
    </button>
  {/if}
</form>
