/**
 * Matching a typed payee name against existing payee records.
 *
 * T-44: a typed name that names no payee used to be stored as free text, and
 * every such transaction collapsed into one "no payee recorded" row in the
 * spending report. The backend now links an *exact* match on its own; this
 * module covers the rest — recognising when a name is genuinely new, and
 * finding the near matches worth offering before a new record is created.
 */

import MiniSearch from 'minisearch';
import type { PayeeResponse } from '$lib/api/payees';

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

export type PayeeResolution =
  | { kind: 'none' }
  | { kind: 'linked'; payee: PayeeResponse }
  | { kind: 'unknown'; name: string; suggestions: PayeeResponse[] };

/**
 * What a typed name means right now.
 *
 * `none` — nothing typed, so the transaction simply has no payee.
 * `linked` — a record already carries this name; saving needs no confirmation.
 * `unknown` — no record carries it, so creating one has to be confirmed and the
 * near matches are worth showing first.
 */
export function resolveTypedPayee(
  name: string,
  selectedPayeeID: number | undefined,
  payees: PayeeResponse[]
): PayeeResolution {
  if (selectedPayeeID !== undefined) {
    const selected = payees.find((payee) => payee.id === selectedPayeeID);
    if (selected) return { kind: 'linked', payee: selected };
  }

  if (normalizePayeeName(name) === '') return { kind: 'none' };

  const exact = exactPayeeMatch(name, payees);
  if (exact) return { kind: 'linked', payee: exact };

  return { kind: 'unknown', name: name.trim(), suggestions: payeeSuggestions(name, payees) };
}

/**
 * Whether saving should stop and ask.
 *
 * Only when the name is new *and* the user changed it. An existing transaction
 * may already carry an unlinked name from before this rule existed, and
 * blocking an edit to an unrelated field over history the user did not create
 * would be punishing them for the old behaviour.
 */
export function needsPayeeConfirmation(resolution: PayeeResolution, nameChanged: boolean): boolean {
  return resolution.kind === 'unknown' && nameChanged;
}
