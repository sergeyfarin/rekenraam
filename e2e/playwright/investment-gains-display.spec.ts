import { expect, test } from '@playwright/test';
import { readyForLedger } from './support/ledger';
import { todayISO } from './support/dates';

// Controlled read-model fixtures exercise rendering independently of allocation
// arithmetic, which has separate service/HTTP oracle tests.
test('gains show currency units, retain share precision and render same-day sales', async ({ page }) => {
  await readyForLedger(page);
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  let requests = 0;
  await page.route('**/api/v1/investments/gains*', (route) => {
    requests++;
    return route.fulfill({ json: {
    currencies: [
      { id: 1, code: 'EUR', standard_scale: 2 },
      { id: 2, code: 'JPY', standard_scale: 0 },
      { id: 3, code: 'KWD', standard_scale: 3 }
    ],
    realized: (requests === 1 ? [1, 2] : [2]).map((transaction_id) => ({
      transaction_id, account_id: 99, commodity_id: 98, cost_commodity_id: 1,
      disposal_date: todayISO(), quantity_value: '1234567', quantity_scale: 6,
      proceeds_value: 1, proceeds_scale: 0, disposed_basis_value: 995,
      disposed_basis_scale: 3, realized_gain_value: 5, realized_gain_scale: 3
    })),
    // Two half-cent gains round individually to 0.01, but their exact total
    // is 0.01, not 0.02. Never re-sum the rounded rows in the client.
    realized_totals: [{ cost_commodity_id: 1, total_gain_value: 10, total_gain_scale: 3 }],
    unrealized: (requests === 1 ? [1, 2, 3] : [2, 3]).map((cost_commodity_id) => ({
      account_id: 99, commodity_id: 98, cost_commodity_id,
      quantity_value: '1234567', quantity_scale: 6,
      remaining_cost_basis_value: 1234567, remaining_cost_basis_scale: 6,
      market_value_value: 3, market_value_scale: 0,
      unrealized_gain_value: 1765433, unrealized_gain_scale: 6
    }))
  }});
  });
  await page.goto('/app/investments');
  await page.getByRole('button', { name: 'Gains', exact: true }).click();
  await expect(page.locator('tbody tr')).toHaveCount(5);
  await expect(page.getByRole('cell', { name: '1.234567', exact: true })).toHaveCount(5);
  await expect(page.getByRole('cell', { name: 'EUR 1.23', exact: true })).toBeVisible();
  await expect(page.getByRole('cell', { name: 'JPY 1', exact: true })).toBeVisible();
  await expect(page.getByRole('cell', { name: 'KWD 1.235', exact: true })).toBeVisible();
  await expect(page.locator('tfoot')).toContainText('EUR 0.01');
  // Exercise keyed-list updates too: duplicate keys can leave stale rows
  // behind even when the initial production render appears correct.
  await page.locator('input[type="date"]').first().fill(todayISO());
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(page.locator('tbody tr')).toHaveCount(3);
  await expect(page.getByRole('cell', { name: '1.234567', exact: true })).toHaveCount(3);
  expect(errors).toEqual([]);
});
