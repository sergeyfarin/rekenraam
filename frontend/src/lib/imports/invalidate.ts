import type { QueryClient } from '@tanstack/svelte-query';
import { attentionQueryKey } from '#lib/api/attention.ts';
import { homeQueryKey } from '#lib/api/home.ts';
import { importHistoryQueryKey, importDividendReviewsQueryKey } from '#lib/api/imports.ts';
import { ledgerQueryKey } from '#lib/api/ledger.ts';

// Import writes go through raw fetch (multipart uploads and the import
// helpers), so the shell's apiClient write middleware never sees them. Every
// import write therefore refreshes the reads that count or list import work
// itself: attention and Home's summaries, the batch history, the global
// Dividends queue and the Ledger reads (#567 review).
export const importMutationQueryKeys = [
  attentionQueryKey,
  homeQueryKey,
  importHistoryQueryKey,
  importDividendReviewsQueryKey,
  ledgerQueryKey
] as const;

export async function invalidateImportReads(queryClient: QueryClient): Promise<void> {
  await Promise.all(importMutationQueryKeys.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
}
