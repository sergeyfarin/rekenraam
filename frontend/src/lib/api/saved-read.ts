import type { QueryClient, QueryKey } from '@tanstack/svelte-query';

/**
 * Puts a fresh read of the saved state into the cache after a failed write
 * (bug class 86, #661).
 *
 * `read` is a request of its own, not a cached refetch: another refresh can
 * cancel a refetch, and a cancelled one reverts to the state from before the
 * write. A cached fetch that started before this read can still resolve after
 * it with an older answer, so those are cancelled before the saved state is
 * written. A fetch started later is at least as fresh and simply refetches.
 */
export async function applySavedRead<T>(
  queryClient: Pick<QueryClient, 'cancelQueries' | 'setQueryData'>,
  queryKey: QueryKey,
  read: () => Promise<T>
): Promise<T> {
  const saved = await read();
  await queryClient.cancelQueries({ queryKey, exact: true });
  queryClient.setQueryData(queryKey, saved);
  return saved;
}
