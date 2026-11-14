// Re-reading a batch the Dividends view changed (#567 review): the Files
// working copy takes the server's rows, keeps the owner's resolutions and
// match choices for rows that are still pending, and reports whether a row
// stopped being pending, which invalidates reviews computed for the old set.

type ResyncRow = { id: number; commit_status: string };

export function isPendingRow(row: ResyncRow): boolean {
  return row.commit_status === 'pending' || row.commit_status === 'failed';
}

export function mergeResync<Row extends ResyncRow, Resolution, Choice>(
  previous: readonly ResyncRow[],
  refreshed: readonly Row[],
  resolutions: ReadonlyMap<number, Resolution>,
  choices: ReadonlyMap<number, Choice>,
  parse: (row: Row) => Resolution
): { changed: boolean; resolutions: Map<number, Resolution>; choices: Map<number, Choice> } {
  const pending = new Set(refreshed.filter(isPendingRow).map((row) => row.id));
  const changed = previous.some((row) => isPendingRow(row) && !pending.has(row.id));
  return {
    changed,
    resolutions: new Map(refreshed.map((row) => [row.id, pending.has(row.id) ? (resolutions.get(row.id) ?? parse(row)) : parse(row)])),
    choices: new Map([...choices].filter(([rowID]) => pending.has(rowID)))
  };
}
