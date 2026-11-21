/**
 * Matching a typed payee name against existing payee records.
 *
 * T-50: a typed name that names no payee used to be stored as free text, and
 * every such transaction collapsed into one "no payee recorded" row in the
 * spending report. The backend now links an *exact* match on its own; this
 * module matches names against the catalog available to import review.
 */

import MiniSearch from 'minisearch';
import type { PayeeResponse } from '#lib/api/payees.ts';

/** The same normalization payee records are stored under, mirrored here. */
export function normalizePayeeName(value: string): string {
  return value.trim().replaceAll(/\s+/gu, ' ').toLocaleLowerCase();
}

/** The payee whose name matches exactly once normalized, if there is one. */
export function exactPayeeMatch(name: string, payees: PayeeResponse[]): PayeeResponse | undefined {
  const normalized = normalizePayeeName(name);
  if (normalized === '') return undefined;
  return payees.find((payee) => normalizePayeeName(payee.name) === normalized);
}

const MAX_SUGGESTIONS = 5;

/**
 * Near matches for a name no payee has, best first.
 *
 * Fuzzy rather than substring, because the case this exists for is a typo or a
 * word order the user did not use last time — "Markt Hall" and "Hall Market"
 * both need to find "Market Hall", and neither contains it.
 */
export function payeeSuggestions(name: string, payees: PayeeResponse[]): PayeeResponse[] {
  const query = name.trim();
  if (query === '' || payees.length === 0) return [];

  const index = new MiniSearch<PayeeResponse>({
    fields: ['name'],
    storeFields: ['id'],
    searchOptions: {
      // A payee list is small, so a wide net costs nothing, and a near miss
      // being absent is exactly what pushes someone into creating a duplicate.
      prefix: true,
      fuzzy: 0.3
    }
  });
  index.addAll(payees);

  const byID = new Map(payees.map((payee) => [payee.id, payee]));
  const matches: PayeeResponse[] = [];
  for (const result of index.search(query)) {
    const payee = byID.get(result.id as number);
    if (payee) matches.push(payee);
    if (matches.length === MAX_SUGGESTIONS) break;
  }
  return matches;
}
