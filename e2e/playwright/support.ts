import { expect, type Page } from '@playwright/test';

// Shared helpers for the browser e2e specs.
//
// ── Dates ────────────────────────────────────────────────────────────────
// Do NOT hard-code a calendar date for a transaction, statement, or imported
// row. Everything a spec seeds — the currency it enables during setup, the
// accounts it creates — is versioned as effective from *today*, and posting
// validation resolves the account and commodity rules **as of the entry date**
// (`PostingAccountRule` / `PostingCommodityRule`). A date that is in the past
// relative to that seeded data therefore matches no version, and the posting is
// rejected with "posting account is invalid".
//
// A hard-coded date is fine on the day it is written and becomes a time bomb
// the moment the wall clock passes it. Exactly that happened to `2026-07-08`,
// which silently broke every browser transaction-entry journey.
//
// Anchor entry dates to the run instead.
export function todayISO(): string {
  return new Date().toISOString().slice(0, 10);
}

// QIF writes dates as MM/DD/YYYY. The importer's locale detection only ever
// resolves an ambiguous pair to MM/DD, and an unambiguous one (day > 12) pins
// MM/DD too, so generating in this order always round-trips to today.
export function todayQIF(): string {
  const now = new Date();
  const month = String(now.getUTCMonth() + 1).padStart(2, '0');
  const day = String(now.getUTCDate()).padStart(2, '0');
  return `${month}/${day}/${now.getUTCFullYear()}`;
}

// ── Assertions ───────────────────────────────────────────────────────────
// Assert that a transaction was saved, by its description.
//
// Two traps make the obvious `getByText(description)` wrong:
//
//  1. Saving opens a detail panel, so the description is on screen up to three
//     times at once (list row, panel heading, panel description). A bare text
//     locator matches all of them and fails Playwright strict mode — which
//     reads as "not saved" when it was saved and is visible three times over.
//  2. The list row shows the *payee* rather than the description when the
//     transaction has one, so asserting against the row only works for
//     transactions entered without a payee.
//
// The detail panel's description field is the one element present in every
// case, and exactly once.
export async function expectSavedTransaction(page: Page, description: string): Promise<void> {
  await expect(page.getByRole('definition').filter({ hasText: description })).toBeVisible();
}
