/**
 * Ledger dates for e2e runs.
 *
 * Accounts created during a run get `effective_from` set to the run date, and
 * `PostingAccountRule` resolves the account version as-of the posting's entry
 * date. A hard-coded posting date therefore starts failing with
 * `VALIDATION_FAILED: posting account is invalid` as soon as wall-clock time
 * passes it — so every posting date must be derived from today.
 */

function pad(n: number): string {
  return String(n).padStart(2, '0');
}

/** Today as `YYYY-MM-DD`, the format the date inputs and API expect. */
export function todayISO(date: Date = new Date()): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** Today as `MM/DD/YYYY`, the format QIF `D` records use. */
export function todayQIF(date: Date = new Date()): string {
  return `${pad(date.getMonth() + 1)}/${pad(date.getDate())}/${date.getFullYear()}`;
}

/** First day of the current month as `YYYY-MM-DD`, for `opened_on` values. */
export function monthStartISO(date: Date = new Date()): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-01`;
}
