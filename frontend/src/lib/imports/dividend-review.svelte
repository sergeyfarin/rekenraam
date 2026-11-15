<script lang="ts">
  import { createInfiniteQuery, createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { SvelteSet } from 'svelte/reactivity';
  import { page } from '$app/state';
  import { importViewURL } from './views.ts';
  import { untrack } from 'svelte';
  import PageHeader from '#lib/components/page-header.svelte';
  import Panel from '#lib/components/panel.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions } from '#lib/api/currencies.ts';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import { investmentInstrumentsQueryOptions } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from '#lib/investments/invalidate.ts';
  import { getImportDividendReviews, importDividendReviewsQueryKey } from '#lib/api/imports.ts';
  import { invalidateImportReads } from './invalidate.ts';
  import DividendReviewRow from './dividend-review-row.svelte';
  import { m } from '#lib/paraglide/messages.js';
  // onSaved tells an embedding Import screen that a batch row changed under
  // its Files working copy.
  // held and onPending serialize its saves with the embedding screen's other
  // batch writes (#567 review). active is false while an embedding screen
  // keeps this view mounted but hidden: its reads pause, and an invalidated
  // read refetches when the view shows again.
  let { batchID, embedded = false, onSaved: onRowSaved, held = false, onPending, active = true }: { batchID?: number; embedded?: boolean; onSaved?: () => void; held?: boolean; onPending?: (pending: boolean) => void; active?: boolean } = $props();
  const client = useQueryClient();
  const session = createQuery(() => authSessionQueryOptions());
  const accounts = createQuery(() => ({ ...accountsQueryOptions(), enabled: active }));
  const currencies = createQuery(() => ({ ...currenciesQueryOptions(), enabled: active }));
  const instruments = createQuery(() => ({ ...investmentInstrumentsQueryOptions(), enabled: active }));
  const reviews = createInfiniteQuery(() => ({
    queryKey: [...importDividendReviewsQueryKey, batchID], initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => getImportDividendReviews(batchID, pageParam), getNextPageParam: (last) => last.next_cursor ?? undefined,
    enabled: active
  }));
  const rows = $derived(reviews.data?.pages.flatMap(page => page.rows) ?? []);
  const loading = $derived(reviews.isPending || accounts.isPending || currencies.isPending || instruments.isPending);
  const loadError = $derived(reviews.error ?? accounts.error ?? currencies.error ?? instruments.error);
  // Back returns to the Files view of the same batch, as the view selector
  // does, so the staged batch and its unsaved choices stay (#567 review).
  const backHref = $derived.by(() => {
    if (embedded) return importViewURL(page.url, 'files');
    const url = new URL('/app/ledger/import', page.url.origin);
    if (batchID) url.searchParams.set('batch_id', String(batchID));
    return importViewURL(url, 'files');
  });
  let saved = $state<{ id: number; repaired: boolean } | null>(null);
  const finished = new SvelteSet<number>();
  // An embedding screen reuses this view across batches: what was recorded
  // for one batch is not shown for the next.
  $effect.pre(() => {
    void batchID;
    untrack(() => { saved = null; finished.clear(); });
  });
  // Rows with a save in flight: one row's save holds the others.
  const saving = new SvelteSet<number>();
  $effect(() => {
    onPending?.(saving.size > 0);
    return () => onPending?.(false);
  });
  async function onSaved(rowID: number, id: number, repaired: boolean) {
    saved = { id, repaired }; finished.add(rowID);
    onRowSaved?.();
    // A recorded dividend changes the batch row, the attention counts and
    // Home as well as the queue; the import helpers bypass the shell's write
    // middleware, so refresh them here.
    await Promise.all([invalidateInvestmentReads(client), invalidateImportReads(client)]);
  }
</script>
{#if embedded}<h2 class="text-lg font-semibold">{m.import_dividend_title()}</h2><p class="mt-2 text-sm text-muted">{m.import_dividend_help()}</p>{:else}<PageHeader title={m.import_dividend_title()} copy={m.import_dividend_help()} />{/if}
<div class="mt-6 max-w-3xl space-y-5">
  <a href={backHref} class="inline-flex min-h-11 items-center text-sm font-semibold text-foreground underline underline-offset-2">{m.import_dividend_back()}</a>
  {#if saved}
    <Panel>
      <p role="status" class="text-sm text-positive">{saved.repaired ? m.import_dividend_repaired() : m.import_dividend_recorded()}</p>
      <a href={`/app/ledger/transactions?transaction_id=${saved.id}`} class="inline-flex min-h-11 items-center text-sm font-semibold text-foreground underline underline-offset-2">{m.import_dividend_view()}</a>
    </Panel>
  {/if}
  {#if loading}
    <p role="status" class="text-sm text-muted">{m.import_dividend_loading()}</p>
  {:else if loadError}
    <APIFormError error={loadError} />
    <button type="button" class="min-h-11 text-sm text-foreground underline" onclick={() => client.invalidateQueries()}>{m.install_gate_retry()}</button>
  {:else if rows.filter(row => !finished.has(row.row_id)).length === 0 && !reviews.hasNextPage}
    <Panel><p role="status" class="text-sm text-muted">{m.import_dividend_empty()}</p></Panel>
  {:else}
    {#each rows.filter(row => !finished.has(row.row_id)) as review (review.row_id)}
      <DividendReviewRow {review} accounts={accounts.data?.accounts ?? []} currencies={currencies.data?.currencies ?? []} instruments={instruments.data?.instruments ?? []} csrfToken={session.data?.csrf_token ?? ''} held={held || (saving.size > 0 && !saving.has(review.row_id))} onPending={(value) => { if (value) saving.add(review.row_id); else saving.delete(review.row_id); }} onSaved={(id, repaired) => onSaved(review.row_id, id, repaired)} />
    {/each}
  {/if}
  {#if reviews.hasNextPage}
    <button type="button" disabled={reviews.isFetchingNextPage} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm text-foreground" onclick={() => reviews.fetchNextPage()}>{m.import_dividend_more()}</button>
  {/if}
</div>
