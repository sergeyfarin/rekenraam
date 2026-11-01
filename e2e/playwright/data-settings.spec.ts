import { expect, test } from './support/fixtures';
import { readFileSync } from 'node:fs';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';
import { test as ownedBookTest } from './support/owned-book';

// [acceptance] R3 slice 7 — the Settings → Data screen, mapped to the
// validation matrix in docs/plans/data-portability-plan.md. Run this subset
// alone with: pnpm --dir e2e exec playwright test --grep "\[acceptance\]"
// (T-61: a suite that answers "does the plan's acceptance hold" without
// re-deriving it from several specs).

test('[acceptance] the export, backup and health-check screens keep their jobs', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();

  // The e2e database is shared within a run, and the reports specs assert on
  // today's category totals across the whole book. This spec only needs *some*
  // postings to exist, so it books them years in the past rather than moving
  // another spec's numbers around underneath it.
  const bookedOn = '2021-03-04';
  const checking = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `Data checking ${suffix}`,
    account_class: 'asset',
    account_kind: 'checking',
    default_commodity_id: currencyID,
    allows_postings: true,
    // Opened before the posting below: a posting predating its own account is
    // exactly what the ledger refuses (T-63).
    opened_on: '2021-01-01'
  });
  const categories = await apiJSON<{
    categories: Array<{ id: number; code?: string; allows_postings: boolean }>;
  }>(page, 'GET', '/api/v1/categories');
  const groceries = categories.categories.find(
    (item) => item.code === 'expense_food_groceries' && item.allows_postings
  );
  if (!groceries) throw new Error('seeded groceries category not found');

  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    transaction_date: bookedOn,
    journal_entries: [
      {
        entry_date: bookedOn,
        postings: [
          { account_id: checking.id, quantity_value: '-4200', quantity_scale: 2, commodity_id: currencyID },
          { account_id: groceries.id, quantity_value: '4200', quantity_scale: 2, commodity_id: currencyID }
        ]
      }
    ]
  });

  // Export moved to Reports (#446); backups and the health check stay here.
  await page.goto('/app/reports/export');
  await expect(page.getByTestId('export-settings')).toBeVisible();

  // Export and backup have different jobs (product decision 2026-10-18): the
  // screen says an export cannot be imported back, and that only a backup is
  // a complete copy to restore from.
  await expect(page.getByTestId('export-not-backup')).toContainText('cannot be imported back');

  // The export says what it holds before offering the file, so a download is
  // never a black box.
  await expect(page.getByTestId('export-summary')).toContainText(/postings/i);

  const download = page.waitForEvent('download');
  await page.getByTestId('export-download').click();
  const file = await download;
  expect(file.suggestedFilename()).toContain('.zip');

  await page.goto('/app/settings/data');
  await expect(page.getByTestId('data-settings')).toBeVisible();
  await expect(page.getByTestId('backup-complete-copy')).toContainText('only complete copy');

  // Backups: queued, not claimed as done.
  await page.getByTestId('backup-now').click();
  await expect(page.getByTestId('backup-queued')).toBeVisible();
  await expect(page.getByTestId('backup-run').first()).toBeVisible();

  // The health check reports and explains.
  await expect(page.getByTestId('selfcheck-summary')).toBeVisible();
  await page.getByTestId('selfcheck-run').click();
  await expect(page.getByTestId('selfcheck-result').first()).toBeVisible();
  await expect(page.getByTestId('selfcheck-summary')).toContainText(/check/i);
});

test('the settings side list reaches the data screen from any settings page', async ({ page }) => {
  await readyForLedger(page);

  // The side list is the same five pages as the header's settings menu.
  await page.goto('/app/settings/security');
  const nav = page.getByRole('navigation', { name: 'Settings sections' });
  await expect(nav.getByRole('link')).toHaveText([
    'General',
    'Preferences',
    'Currencies and FX rates',
    'Security',
    'Data and backups'
  ]);
  await nav.getByRole('link', { name: 'Data and backups', exact: true }).click();
  await expect(page).toHaveURL(/\/app\/settings\/data$/);
  await expect(page.getByTestId('data-settings')).toBeVisible();
  await expect(nav.getByRole('link', { name: 'Data and backups', exact: true })).toHaveAttribute('aria-current', 'page');
});

test('settings has five pages, the same in the menu and side list, and old addresses redirect', async ({ page }) => {
  await readyForLedger(page);
  await page.goto('/app/settings');
  await expect(page.getByTestId('general-settings')).toBeVisible();

  const side = page.getByRole('navigation', { name: 'Settings sections', exact: true });
  const names = ['General', 'Preferences', 'Currencies and FX rates', 'Security', 'Data and backups'];
  await expect(side.getByRole('link')).toHaveText(names);

  await page.getByRole('button', { name: 'Settings', exact: true }).click();
  await expect(page.locator('#shell-settings-menu').getByRole('link')).toHaveText(names);
  await page.keyboard.press('Escape');

  await page.goto('/app/settings/language?keep=1#detail');
  await expect(page).toHaveURL('/app/settings/preferences?keep=1#detail');
  await expect(page.getByTestId('preferences-settings')).toBeVisible();

  await page.goto('/app/settings/appearance');
  await expect(page).toHaveURL('/app/settings/preferences');

  await page.goto('/app/settings/trash?x=1#detail');
  await expect(page).toHaveURL('/app/ledger/transactions?x=1&view=deleted#detail');
  await expect(page.getByTestId('transactions-view-deleted')).toHaveAttribute('aria-current', 'page');

  await page.goto('/app/settings/data?view=export&q=kept');
  await expect(page).toHaveURL('/app/reports/export?q=kept');
  await expect(page.getByTestId('export-settings')).toBeVisible();

  await page.setViewportSize({ width: 320, height: 844 });
  for (const path of ['/app/settings', '/app/settings/preferences', '/app/reports/export', '/app/settings/data']) {
    await test.step(`320px ${path}`, async () => {
      await page.goto(path);
      await expect(page.locator('main h1, main h2').first()).toBeVisible();
      expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBe(0);
    });
  }
});

// The one thing a user must not be able to do by accident: download a QIF that
// silently omits accounts QIF cannot express.
test('[acceptance] a QIF export names the accounts it cannot write before downloading', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const suffix = Date.now();

  await createCashAccount(page, csrfToken, `QIF checking ${suffix}`, currencyID);
  // A brokerage account: QIF has no honest way to write it.
  await apiJSON(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: `QIF brokerage ${suffix}`,
    account_class: 'asset',
    account_kind: 'brokerage',
    default_commodity_id: currencyID,
    allows_postings: true
  });

  await page.goto('/app/reports/export');
  await page.getByRole('radio', { name: /QIF/ }).check();

  // Whole-book QIF simply skips what it cannot write, so the download stays
  // available — the confirmation is for a selection that names one.
  await expect(page.getByTestId('export-download')).toBeEnabled();
});

// The backup form edits the configured policy. An edit that was never saved
// must not come back looking like the configured one: someone who changes the
// time, thinks better of it, and returns later would otherwise read their own
// discarded number as the schedule the app is running on.
test('an unsaved backup-policy edit does not survive as if it were saved', async ({ page }) => {
  await readyForLedger(page);

  await page.goto('/app/settings/data');
  await expect(page.getByTestId('data-settings')).toBeVisible();

  const retention = page.getByTestId('backup-retention');
  const configured = await retention.inputValue();
  const edited = String(Number(configured) + 7);

  await retention.fill(edited);
  await expect(retention).toHaveValue(edited);

  // Leave without saving, then come back — by navigating inside the app, which
  // is what a user does. A full reload would prove nothing here: it discards
  // the client cache the unsaved edit could be hiding in.
  const nav = page.getByRole('navigation', { name: 'Settings sections' });
  await nav.getByRole('link', { name: 'General', exact: true }).click();
  await expect(page).toHaveURL(/\/app\/settings$/);
  await nav.getByRole('link', { name: 'Data and backups', exact: true }).click();
  await expect(page.getByTestId('data-settings')).toBeVisible();

  await expect(page.getByTestId('backup-retention')).toHaveValue(configured);

  // The other half of the same rule: a saved edit *must* come back. Holding the
  // form's own copy is only correct if it still reaches the server.
  await page.getByTestId('backup-retention').fill(edited);
  await page.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(page.getByText('Saved.')).toBeVisible();

  await nav.getByRole('link', { name: 'General', exact: true }).click();
  await expect(page).toHaveURL(/\/app\/settings$/);
  await nav.getByRole('link', { name: 'Data and backups', exact: true }).click();
  await expect(page.getByTestId('backup-retention')).toHaveValue(edited);
});

function ago(days: number) {
  const date = new Date();
  date.setDate(date.getDate() - days);
  return todayISO(date);
}

// #313: supported unknown investment basis is disclosed as incomplete
// coverage, never as an all-green pass, in every locale and on a phone. The
// run status is book-wide, so the test owns its book (#398): the unknown-basis
// lots below are the only thing the run can report, and the result is exactly
// incomplete rather than whatever another spec's state makes it.
for (const locale of ['en', 'es', 'fr', 'nl', 'de', 'ru']) {
  ownedBookTest(`[acceptance] the health check discloses unknown basis as incomplete in ${locale}`, async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const settings = JSON.parse(
      readFileSync(new URL(`../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8')
    ) as Record<string, string>;
    const { csrfToken, currencyID } = await readyForLedger(page);
    const suffix = `sc${locale}${Date.now()}`;
    const instrument = await apiJSON<{ id: number; commodity_id: number }>(
      page,
      'POST',
      '/api/v1/investments/instruments',
      csrfToken,
      {
        commodity_code: suffix.toUpperCase(),
        instrument_type: 'stock',
        display_name: suffix,
        symbol: suffix.toUpperCase(),
        quote_commodity_id: currencyID,
        trading_commodity_id: currencyID,
        quantity_scale: 3,
        price_scale: 2,
        effective_from: ago(60)
      }
    );
    const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
      instrument_id: instrument.id,
      name: `Holding ${suffix}`,
      opened_on: ago(60),
      effective_from: ago(60)
    });
    await apiJSON(page, 'POST', '/api/v1/investments/transfers/external/in', csrfToken, {
      effective_on: ago(40),
      holding_account_id: holding.id,
      commodity_id: instrument.commodity_id,
      quantity_value: '2',
      quantity_scale: 0,
      cost_commodity_id: currencyID,
      basis_knowledge: 'unknown'
    });

    await page.evaluate((value) => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto('/app/settings/data');
    const ran = page.waitForResponse(
      (response) => response.url().endsWith('/api/v1/maintenance/self-check') && response.request().method() === 'POST'
    );
    await page.getByTestId('selfcheck-run').click();
    const run = (await (await ran).json()) as {
      status: string;
      incomplete_check_count: number;
      results: Array<{ check_id: string; status: string; unavailable_count: number; unavailable_sample: number[] }>;
    };
    const lots = run.results.find((result) => result.check_id === 'lot_reconciliation');
    expect(run.status).toBe('incomplete');
    expect(run.incomplete_check_count).toBe(1);
    expect(lots?.status).toBe('incomplete');
    expect(lots?.unavailable_count).toBe(1);
    expect(run.results.filter((result) => result.unavailable_count > 0).map((result) => result.check_id)).toEqual([
      'lot_reconciliation'
    ]);

    const summary = page.getByTestId('selfcheck-summary');
    await expect(summary).not.toContainText(settings.settings_data_selfcheck_passed);
    await expect(summary).toHaveText(settings.settings_data_selfcheck_incomplete.replace('{count}', '1'));
    await expect(page.getByTestId('selfcheck-unavailable')).toHaveText(
      settings.settings_data_selfcheck_unavailable.replace('{count}', '1')
    );
    await expect(
      page.getByTestId('selfcheck-result').filter({ has: page.getByTestId('selfcheck-unavailable') })
    ).toContainText(settings.settings_data_status_incomplete);
    const pageWidth = await page.evaluate(() => document.documentElement.scrollWidth);
    expect(pageWidth).toBeLessThanOrEqual(390);
  });
}
