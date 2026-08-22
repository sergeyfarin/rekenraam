/**
 * Ledger prerequisites for e2e runs.
 *
 * Postings need the system accounts and the seeded category tree, and every
 * account needs a commodity — so specs that touch money start from
 * `readyForLedger` rather than assuming a seeded database.
 */

import { expect, type Page } from '@playwright/test';
import { apiJSON, csrfTokenFor } from './api';
import { ensureBrowserSession } from './session';

/**
 * Signs in, seeds system accounts and categories, and returns a fresh CSRF
 * token plus the currency to book in (USD when present).
 *
 * Seeding is idempotent: a second run gets 409 from both setup endpoints, and
 * both statuses are accepted.
 */
export async function readyForLedger(page: Page): Promise<{ csrfToken: string; currencyID: number }> {
  await ensureBrowserSession(page);
  let csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', '/api/v1/setup/system-accounts', csrfToken, undefined, [201, 409]);
  csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', '/api/v1/setup/categories', csrfToken, undefined, [201, 409]);

  const currencies = await apiJSON<{ currencies: Array<{ id: number; code: string }> }>(page, 'GET', '/api/v1/currencies');
  const currency = currencies.currencies.find((item) => item.code === 'USD') ?? currencies.currencies[0];
  if (!currency) throw new Error('expected a configured currency');

  return { csrfToken: await csrfTokenFor(page), currencyID: currency.id };
}

/** Creates a posting-enabled checking account. */
export async function createCashAccount(
  page: Page,
  csrfToken: string,
  name: string,
  currencyID: number
): Promise<{ id: number; name: string }> {
  return apiJSON(page, 'POST', '/api/v1/accounts', csrfToken, {
    name,
    account_class: 'asset',
    account_kind: 'checking',
    default_commodity_id: currencyID,
    allows_postings: true
  });
}

/** Creates a posting-enabled credit-card account, outside any liquid-cash scope. */
export async function createLiabilityAccount(
  page: Page,
  csrfToken: string,
  name: string,
  currencyID: number
): Promise<{ id: number; name: string }> {
  return apiJSON(page, 'POST', '/api/v1/accounts', csrfToken, {
    name,
    account_class: 'liability',
    account_kind: 'credit_card',
    default_commodity_id: currencyID,
    allows_postings: true
  });
}

/**
 * Assert that a transaction was saved, by its description.
 *
 * Two traps make the obvious `getByText(description)` wrong:
 *
 *  1. Saving opens a detail panel, so the description is on screen up to three
 *     times at once (list row, panel heading, panel description). A bare text
 *     locator matches all of them and fails Playwright strict mode — which
 *     reads as "not saved" when it was saved and is visible three times over.
 *  2. The list row shows the *payee* rather than the description when the
 *     transaction has one, so asserting against the row only works for
 *     transactions entered without a payee.
 *
 * The detail panel's description field is the one element present in every
 * case, and exactly once.
 */
export async function expectSavedTransaction(page: Page, description: string): Promise<void> {
  await expect(page.getByRole('definition').filter({ hasText: description })).toBeVisible();
}
