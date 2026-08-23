/**
 * How a failed report request is presented.
 *
 * Reports are read-only, so the honest response to most failures is "try
 * again". Not to all of them: `LEDGER_OVERFLOW` means an aggregate exceeded the
 * exact 38-digit coefficient limit, and exact arithmetic gives the same answer
 * every time — offering retry as the only way out would be advice that cannot
 * work. Where the API returned a stable code, the localized message for that
 * code is shown instead of the view's generic copy, and retry is offered only
 * when repeating the identical request could plausibly succeed.
 */

import type { APIErrorCode } from '$lib/api/client';
import { APIClientError } from '$lib/api/client';
import { getAPIErrorMessage } from '$lib/api-error-messages';

/**
 * Codes an identical repeat of the same report query cannot resolve.
 *
 * `UNAUTHENTICATED` and `CSRF_INVALID` belong here for the same reason as the
 * rest: the request that failed carries the credential that failed, so sending
 * it again sends the same expired session. Recovery is the app shell's job —
 * it re-reads the session and returns the reader to sign-in — not a button that
 * repeats a request already known to be refused.
 */
const PERMANENT_CODES = new Set<APIErrorCode>([
  'VALIDATION_FAILED',
  'UNAUTHENTICATED',
  'FORBIDDEN',
  'NOT_FOUND',
  'CONFLICT',
  'CSRF_INVALID',
  'LEDGER_OVERFLOW',
  'CONFIG_REQUIRED',
  'SETUP_REQUIRED',
  'SETUP_ALREADY_COMPLETE'
]);

export type ReportErrorState = {
  copy: string;
  retryable: boolean;
};

/**
 * Whether repeating the identical report request could plausibly succeed.
 *
 * Kept separate from the message lookup so the decision is testable: unit tests
 * run outside a locale, and asking paraglide for a translation there throws.
 */
export function isRetryableAPIError(code: APIErrorCode): boolean {
  return !PERMANENT_CODES.has(code);
}

/** The code a failed report request carries, or undefined if it has none. */
export function reportErrorCode(error: unknown): APIErrorCode | undefined {
  return error instanceof APIClientError ? error.code : undefined;
}

export function reportErrorState(error: unknown, fallbackCopy: string): ReportErrorState {
  const code = reportErrorCode(error);
  if (code !== undefined) {
    return { copy: getAPIErrorMessage(code), retryable: isRetryableAPIError(code) };
  }
  // A transport failure or an unrecognized shape keeps the view's own copy: it
  // names the report the reader is looking at, which a generic code cannot.
  return { copy: fallbackCopy, retryable: true };
}
