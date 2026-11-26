import { afterNavigate } from '$app/navigation';
import { useQueryClient } from '@tanstack/svelte-query';
import { createContext } from 'svelte';
import { APIClientError } from '#lib/api/client.ts';
import { homeLayoutQueryOptions, homeQueryKey, putHomeLayout, type Home, type HomeLayout, type HomeLayoutState, type HomeWidgetID } from '#lib/api/home.ts';

export type CustomizeTarget = 'all' | 'shortcuts' | 'reset' | HomeWidgetID;
type SaveResult = { status: 'saved' | 'conflict'; state: HomeLayoutState } | { status: 'failed' };

const [useHomePersonalization, provideHomePersonalization] = createContext<ReturnType<typeof createHomePersonalization>>();
export { useHomePersonalization };

export function createHomePersonalization(csrfToken: () => string | undefined) {
  const queryClient = useQueryClient();
  let target = $state<CustomizeTarget | null>(null);
  let pending = $state(false);
  let error = $state<unknown>();
  let conflict = $state(false);
  let undo = $state.raw<{ before: HomeLayout; saved: HomeLayoutState } | null>(null);
  let announcement = $state('');

  afterNavigate(({ to, from }) => {
    if (to?.url.pathname !== from?.url.pathname) {
      target = null;
      error = undefined;
      conflict = false;
      undo = null;
      announcement = '';
    }
  });

  async function updateHome(result: HomeLayoutState) {
    const cached = queryClient.getQueriesData<Home>({ queryKey: homeQueryKey });
    await Promise.all(cached.map(([queryKey, home]) => {
      if (home && home.personalization.revision > result.revision) return;
      const query = queryClient.getQueryState(queryKey);
      if (home && query?.status === 'success' && query.fetchStatus === 'idle' && !query.isInvalidated &&
        sameFigureScopes(home.personalization.layout, result.layout)) {
        // A presentation change can appear immediately. Keep the existing
        // financial read's age and never relabel figures from another scope.
        queryClient.setQueryData(queryKey, { ...home, personalization: result }, { updatedAt: query.dataUpdatedAt });
        return;
      }
      return queryClient.invalidateQueries({ queryKey, exact: true });
    }));
  }

  async function save(before: HomeLayoutState, layout: HomeLayout | null, offerUndo = true): Promise<SaveResult> {
    if (pending) return { status: 'failed' };
    pending = true;
    error = undefined;
    conflict = false;
    announcement = '';
    try {
      const token = csrfToken();
      if (!token) throw new APIClientError({ status: 403, code: 'CSRF_INVALID' });
      const result = await putHomeLayout(before.revision, layout, token);
      queryClient.setQueryData(homeLayoutQueryOptions.queryKey, result);
      undo = offerUndo ? { before: before.layout, saved: result } : null;
      await updateHome(result);
      return { status: 'saved', state: result };
    } catch (err) {
      if (err instanceof APIClientError && err.status === 409) {
        undo = null;
        conflict = true;
        try {
          await queryClient.cancelQueries({ queryKey: homeLayoutQueryOptions.queryKey });
          const [state] = await Promise.all([
            queryClient.fetchQuery(homeLayoutQueryOptions),
            queryClient.invalidateQueries({ queryKey: homeQueryKey })
          ]);
          return { status: 'conflict', state };
        } catch (reloadError) {
          error = reloadError;
        }
      } else {
        error = err;
      }
      return { status: 'failed' };
    } finally {
      pending = false;
    }
  }

  const controller = {
    get target() { return target; },
    get pending() { return pending; },
    get error() { return error; },
    get conflict() { return conflict; },
    get hasUndo() { return undo !== null; },
    get announcement() { return announcement; },
    open(next: CustomizeTarget = 'all') {
      error = undefined;
      conflict = false;
      target = next;
    },
    close() { target = null; },
    save,
    announce(message: string) { announcement = message; },
    async undo() {
      if (undo) await save(undo.saved, undo.before, false);
    }
  };
  provideHomePersonalization(controller);
  return controller;
}

function sameFigureScopes(before: HomeLayout, after: HomeLayout): boolean {
  const sameIDs = (a: number[] = [], b: number[] = []) => a.length === b.length && a.every((id, index) => id === b[index]);
  return before.account_id === after.account_id &&
    sameIDs(before.account_set?.account_ids, after.account_set?.account_ids) &&
    sameIDs(before.widget_options.spending?.account_ids, after.widget_options.spending?.account_ids) &&
    (before.widget_options.cashflow?.period ?? 'this_month') === (after.widget_options.cashflow?.period ?? 'this_month');
}
