import type { ReadonlyURLSearchParams } from '$app/state';
import type { PayeeDirectoryStatus } from '#lib/api/payees.ts';

export type PayeeDirectorySelection = {
  status: PayeeDirectoryStatus;
  q: string;
  cursor?: string;
  previousCursors: string[];
};

export type PayeeDirectoryChange =
  | { kind: 'filter'; q?: string; status?: PayeeDirectoryStatus }
  | { kind: 'next'; cursor: string }
  | { kind: 'previous' };

export function readPayeeDirectorySelection(
  params: ReadonlyURLSearchParams,
  saved?: PayeeDirectorySelection
): PayeeDirectorySelection {
  const statusParam = params.get('status');
  const status = statusParam === 'archived' || statusParam === 'all' ? statusParam : 'active';
  const q = params.get('q')?.trim() ?? '';
  const cursor = params.get('cursor') || undefined;
  // The URL stands on its own. The history entry remembers earlier cursors
  // when this page was reached by paging through these same filters.
  const previousCursors = cursor && saved?.cursor === cursor && saved.status === status && saved.q === q
    ? saved.previousCursors
    : [];
  return { status, q, cursor, previousCursors };
}

export function changePayeeDirectorySelection(
  current: PayeeDirectorySelection,
  change: PayeeDirectoryChange
): PayeeDirectorySelection {
  switch (change.kind) {
    case 'filter': {
      const status = change.status ?? current.status;
      const q = change.q?.trim() ?? current.q;
      return status === current.status && q === current.q ? current : { status, q, previousCursors: [] };
    }
    case 'next':
      return {
        ...current,
        cursor: change.cursor,
        previousCursors: current.cursor ? [...current.previousCursors, current.cursor] : []
      };
    case 'previous':
      return {
        ...current,
        cursor: current.previousCursors.at(-1),
        previousCursors: current.previousCursors.slice(0, -1)
      };
  }
}

export function payeeDirectoryURL(href: string, { status, q, cursor }: PayeeDirectorySelection): URL {
  const url = new URL(href);
  for (const [key, value] of Object.entries({ status: status === 'active' ? undefined : status, q, cursor })) {
    if (value) url.searchParams.set(key, value);
    else url.searchParams.delete(key);
  }
  return url;
}
