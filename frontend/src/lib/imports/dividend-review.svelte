<script lang="ts">
  import { createInfiniteQuery, createQuery, useQueryClient } from '@tanstack/svelte-query';
  import { SvelteSet } from 'svelte/reactivity';
  import PageHeader from '#lib/components/page-header.svelte';
  import Panel from '#lib/components/panel.svelte';
  import APIFormError from '#lib/components/api-form-error.svelte';
  import { accountsQueryOptions } from '#lib/api/accounts.ts';
  import { currenciesQueryOptions } from '#lib/api/currencies.ts';
  import { authSessionQueryOptions } from '#lib/api/auth.ts';
  import { investmentInstrumentsQueryOptions } from '#lib/api/investments.ts';
  import { invalidateInvestmentReads } from '#lib/investments/invalidate.ts';
  import { getImportDividendReviews, importDividendReviewsQueryKey } from '#lib/api/imports.ts';
  import DividendReviewRow from './dividend-review-row.svelte';
  import { m } from '#lib/paraglide/messages.js';
  let { batchID }: { batchID?: number } = $props();
  const client = useQueryClient();
  const session = createQuery(() => authSessionQueryOptions());
  const accounts = createQuery(() => accountsQueryOptions());
  const currencies = createQuery(() => currenciesQueryOptions());
  const instruments = createQuery(() => investmentInstrumentsQueryOptions());
  const reviews = createInfiniteQuery(() => ({
    queryKey: [...importDividendReviewsQueryKey, batchID], initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => getImportDividendReviews(batchID, pageParam), getNextPageParam: (last) => last.next_cursor ?? undefined
  }));
  const rows = $derived(reviews.data?.pages.flatMap(page => page.rows) ?? []);
  const loading = $derived(reviews.isPending || accounts.isPending || currencies.isPending || instruments.isPending);
  const loadError = $derived(reviews.error ?? accounts.error ?? currencies.error ?? instruments.error);
  let saved = $state<{ id: number; repaired: boolean } | null>(null);
  const finished = new SvelteSet<number>();
  async function onSaved(rowID: number, id: number, repaired: boolean) {
    saved = { id, repaired }; finished.add(rowID);
    await invalidateInvestmentReads(client);
    await client.invalidateQueries({ queryKey: importDividendReviewsQueryKey });
  }
</script>
<PageHeader title={m.import_dividend_title()} copy={m.import_dividend_help()} />
<div class="mt-6 max-w-3xl space-y-5">
  <a href="/app/ledger/import" class="inline-flex min-h-11 items-center text-sm font-semibold text-foreground underline underline-offset-2">{m.import_dividend_back()}</a>
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
      <DividendReviewRow {review} accounts={accounts.data?.accounts ?? []} currencies={currencies.data?.currencies ?? []} instruments={instruments.data?.instruments ?? []} csrfToken={session.data?.csrf_token ?? ''} onSaved={(id, repaired) => onSaved(review.row_id, id, repaired)} />
    {/each}
  {/if}
  {#if reviews.hasNextPage}
    <button type="button" disabled={reviews.isFetchingNextPage} class="min-h-11 rounded-(--radius-control) border border-border bg-control px-4 py-2 text-sm text-foreground" onclick={() => reviews.fetchNextPage()}>{m.import_dividend_more()}</button>
  {/if}
</div>
