import type { ImportResolution, ImportStagedRow } from '$lib/api/imports';
import { parseNormalized } from '$lib/api/imports';
import type { PayeeResponse } from '$lib/api/payees';
import { exactPayeeMatch, normalizePayeeName, payeeSuggestions } from '$lib/transactions/payee-matching';

export interface UnknownImportPayeeGroup {
  key: string;
  name: string;
  rowIDs: number[];
  suggestions: PayeeResponse[];
}

/**
 * Groups includable staged rows by unresolved payee name.
 *
 * Exact existing payees need no prompt: the shared transaction validation
 * links those during commit. Everything else is shown once per distinct name
 * so a statement containing the same shop twenty times needs one decision.
 */
export function unknownImportPayeeGroups(
  rows: ImportStagedRow[],
  resolutions: Map<number, ImportResolution>,
  payees: PayeeResponse[]
): UnknownImportPayeeGroup[] {
  const groups = new Map<string, { name: string; rowIDs: number[] }>();

  for (const row of rows) {
    const resolution = resolutions.get(row.id) ?? {};
    if (
      row.dedupe_status === 'duplicate' ||
      row.dedupe_status === 'excluded' ||
      resolution.exclude ||
      resolution.payee_id !== undefined
    ) {
      continue;
    }

    const name = (resolution.payee_name || parseNormalized(row).payee_hint).trim();
    const key = normalizePayeeName(name);
    if (key === '' || exactPayeeMatch(name, payees)) continue;

    const group = groups.get(key);
    if (group) {
      group.rowIDs.push(row.id);
    } else {
      groups.set(key, { name, rowIDs: [row.id] });
    }
  }

  return [...groups.entries()].map(([key, group]) => ({
    key,
    name: group.name,
    rowIDs: group.rowIDs,
    suggestions: payeeSuggestions(group.name, payees)
  }));
}
