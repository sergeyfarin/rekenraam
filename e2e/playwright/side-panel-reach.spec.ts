import type { Page } from '@playwright/test';
import { test, expect } from './support/owned-book';
import { apiJSON } from './support/api';
import { todayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

/**
 * The desktop side panel sticks beside a long list (#515 CI). Stuck at a fixed
 * top, a panel taller than the window kept its bottom — Save transaction —
 * below the screen until the list beside it ended, and infinite scroll moved
 * that end away again; a stuck panel's top also slid under the sticky shell
 * bar. Now a panel that fits sticks below the bar, and a taller one sticks at
 * its own bottom, with one page scroll throughout. Each book is owned, so the
 * list length and the tag count are the test's own.
 */
async function openEntryBesideLongList(page: Page, tags: number) {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const account = await createCashAccount(page, csrfToken, `Reach checking ${Date.now()}`, currencyID);
  const category = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Reach groceries ${Date.now()}`, account_class: 'expense', account_kind: 'expense',
    default_commodity_id: currencyID, allows_postings: true
  });
  // Every tag is a checkbox row in the form.
  for (let i = 0; i < tags; i++) {
    await apiJSON(page, 'POST', '/api/v1/tags', csrfToken, { name: `Reach tag ${i}` }, [201]);
  }
  const today = todayISO();
  for (let i = 0; i < 30; i++) {
    await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
      transaction_date: today, status: 'posted', description: `Reach filler ${i}`,
      journal_entries: [{ entry_date: today, postings: [
        { account_id: account.id, commodity_id: currencyID, quantity_value: '-100', quantity_scale: 2 },
        { account_id: category.id, commodity_id: currencyID, quantity_value: '100', quantity_scale: 2 }
      ] }]
    });
  }
  await page.goto('/app/ledger/transactions');
  await page.getByRole('button', { name: 'New transaction' }).click();
  const form = page.getByRole('region', { name: 'New transaction', exact: true });
  await expect(form.getByRole('heading', { name: 'New transaction' })).toBeVisible();
  const panel = page.getByRole('complementary', { name: 'Actions' }).locator(':scope > div.relative');
  return { form, panel };
}

/** Halfway down the list, where the panel is stuck. */
async function scrollHalfway(page: Page) {
  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight / 2));
  await expect.poll(() => page.evaluate(() => window.scrollY)).toBeGreaterThan(500);
}

test('a panel taller than the window sticks with Save on screen', async ({ page }) => {
  // Twelve tags make New transaction taller than the 720px window.
  const { form, panel } = await openEntryBesideLongList(page, 12);
  const viewport = page.viewportSize()!;
  expect((await panel.boundingBox())!.height, 'the panel must be taller than the window').toBeGreaterThan(viewport.height);

  await scrollHalfway(page);
  const save = form.getByRole('button', { name: 'Save transaction' });
  await expect(save).toBeInViewport();
  expect((await panel.boundingBox())!.y + (await panel.boundingBox())!.height).toBeLessThanOrEqual(viewport.height);
  // The click CI timed out on: actionable where it stands.
  await save.click({ trial: true });
});

test('a panel that fits the window sticks below the shell bar', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 1100 });
  const { form, panel } = await openEntryBesideLongList(page, 0);
  expect((await panel.boundingBox())!.height, 'the panel must fit the window').toBeLessThan(1100 - 140);

  await scrollHalfway(page);
  const shellBottom = (await page.locator('.shell-bar').boundingBox())!.height;
  await expect.poll(async () => (await form.getByRole('heading', { name: 'New transaction' }).boundingBox())!.y)
    .toBeGreaterThanOrEqual(shellBottom);
  await expect(form.getByRole('button', { name: 'Save transaction' })).toBeInViewport();
});
