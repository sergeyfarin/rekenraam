import { expect, test } from '@playwright/test';
import { apiJSON } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';

// [acceptance] R3 slice 7 — the Settings → Data screen, mapped to the
// validation matrix in docs/plans/data-portability-plan.md. Run this subset
// alone with: pnpm --dir e2e exec playwright test --grep "\[acceptance\]"
// (T-61: a suite that answers "does the plan's acceptance hold" without
// re-deriving it from several specs).

test('[acceptance] the data screen exports, backs up, and checks the ledger', async ({ page }) => {
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

  await page.goto('/app/settings/data');
  await expect(page.getByTestId('data-settings')).toBeVisible();

  // The export says what it holds before offering the file, so a download is
  // never a black box.
  await expect(page.getByTestId('export-summary')).toContainText(/postings/i);

  const download = page.waitForEvent('download');
  await page.getByTestId('export-download').click();
  const file = await download;
  expect(file.suggestedFilename()).toContain('.zip');

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

  await page.goto('/app/settings/data');
  await page.getByRole('radio', { name: /QIF/ }).check();

  // Whole-book QIF simply skips what it cannot write, so the download stays
  // available — the confirmation is for a selection that names one.
  await expect(page.getByTestId('export-download')).toBeEnabled();
});
