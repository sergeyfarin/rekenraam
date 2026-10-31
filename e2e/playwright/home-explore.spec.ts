import { test, expect } from './support/owned-book';
import { apiJSON, csrfTokenFor } from './support/api';
import { createCashAccount, readyForLedger } from './support/ledger';
import { expectNoAccessibilityViolations } from './support/a11y';
import { daysFromTodayISO, todayISO } from './support/dates';

// More to explore (#479): every widget not on My Home, at real size, read from
// the same Home read and the shell's attention read. An owned book, because
// every widget is a book-wide summary.

const exploreWidgets = ['cashflow', 'holdings', 'returns', 'allocation', 'income', 'forecast', 'imports', 'prices', 'backups'];

async function bookWithCashflow(page: Parameters<typeof readyForLedger>[0]) {
  const { currencyID } = await readyForLedger(page);
  let csrfToken = await csrfTokenFor(page);
  await apiJSON(page, 'POST', `/api/v1/currencies/${currencyID}/default`, csrfToken, undefined, [200]);
  csrfToken = await csrfTokenFor(page);
  const account = await createCashAccount(page, csrfToken, 'Explore checking', currencyID);
  const salary = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/categories', csrfToken, { name: 'Explore salary', category_type: 'income' });
  const { owner_today: today } = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
  await apiJSON(page, 'POST', '/api/v1/transactions', csrfToken, {
    status: 'posted', transaction_date: today, journal_entries: [{ entry_date: today, postings: [
      { account_id: account.id, commodity_id: currencyID, quantity_value: '250000', quantity_scale: 2 },
      { account_id: salary.id, commodity_id: currencyID, quantity_value: '-250000', quantity_scale: 2 }
    ] }]
  });
  return { currencyID, today };
}

test('More to explore shows every other widget from the same reads, collapses, and drills down', async ({ page }) => {
  const { currencyID, today } = await bookWithCashflow(page);
  const reads: string[] = [];
  page.on('request', request => {
    const path = new URL(request.url()).pathname;
    if (path.startsWith('/api/v1/')) reads.push(path);
  });
  await page.goto('/app');

  const explore = page.getByTestId('home-explore');
  await expect(explore.getByRole('heading', { name: 'More to explore', level: 2 })).toBeVisible();
  await expect(explore.getByText('Add any of the widgets below to the home page')).toBeVisible();
  for (const heading of ['Everyday', 'Wealth & investments', 'Planning', 'Ledger & data']) {
    await expect(explore.getByRole('heading', { name: heading, level: 3 })).toBeVisible();
  }
  for (const id of exploreWidgets) {
    await expect(page.getByTestId(`explore-widget-${id}`)).toBeVisible();
    // A widget is in exactly one place.
    await expect(page.getByTestId(`home-widget-${id}`)).toHaveCount(0);
  }
  const cashflow = page.getByTestId('explore-widget-cashflow');
  await expect(cashflow).toHaveAttribute('data-state', 'ready');
  await expect(cashflow.getByText(/2,500/).first()).toBeVisible();
  await expect(page.getByTestId('explore-widget-returns')).toHaveAttribute('data-state', 'not_set_up');
  await expect(page.getByTestId('explore-widget-forecast').getByText('Estimate')).toBeVisible();
  await expect(page.getByTestId('explore-widget-imports')).toHaveAttribute('data-state', 'zero');

  // Mounting More to explore adds no request: one Home read, the shell's
  // attention read, and no report, returns, forecast, import or backup call.
  await page.waitForLoadState('networkidle');
  expect(reads.filter(path => path === '/api/v1/home')).toHaveLength(1);
  expect(reads.filter(path => /^\/api\/v1\/(reports|budgets|ledger|forecasts|investments|imports|maintenance|backups)(\/|$)/.test(path))).toEqual([]);

  const toggle = explore.getByRole('button', { name: 'Collapse' });
  await expect(toggle).toHaveAttribute('aria-expanded', 'true');
  await toggle.click();
  await expect(explore.getByRole('button', { name: 'Show all' })).toHaveAttribute('aria-expanded', 'false');
  await expect(page.getByTestId('explore-widget-cashflow')).toHaveCount(0);
  await explore.getByRole('button', { name: 'Show all' }).click();
  await expect(page.getByTestId('explore-widget-cashflow')).toBeVisible();
  expect(reads.filter(path => path === '/api/v1/home')).toHaveLength(1);

  await cashflow.getByRole('link', { name: /Open cashflow report/ }).click();
  await expect(page).toHaveURL(new RegExp(`/app/reports/cashflow\\?start_date=${today.slice(0, 8)}01&end_date=${today}&bucket=month&reporting_currency_id=${currencyID}`));
});

for (const theme of ['light', 'dark'] as const) {
  test(`More to explore is accessible and reflows in ${theme}`, async ({ page }) => {
    await bookWithCashflow(page);
    await page.evaluate(value => localStorage.setItem('rekenraam-theme', value), theme);
    await page.goto('/app');
    await expect(page.getByTestId('explore-widget-cashflow')).toHaveAttribute('data-state', 'ready');
    // Muted until hovered, at full colour on focus: the text still passes contrast.
    await expectNoAccessibilityViolations(page, `home explore ${theme} desktop`);
    await page.getByTestId('explore-widget-cashflow').getByRole('link', { name: /Open cashflow report/ }).focus();
    await expectNoAccessibilityViolations(page, `home explore ${theme} focused`);
    await page.setViewportSize({ width: 390, height: 844 });
    await expectNoAccessibilityViolations(page, `home explore ${theme} phone`);
  });
}

test('More to explore reflows at 320px in every locale', async ({ page }) => {
  await bookWithCashflow(page);
  await page.setViewportSize({ width: 320, height: 720 });
  for (const locale of ['en', 'nl', 'de', 'fr', 'es', 'ru']) {
    await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
    await page.goto('/app');
    await expect(page.getByTestId('explore-widget-cashflow')).toHaveAttribute('data-state', 'ready');
    for (const id of exploreWidgets) await expect(page.getByTestId(`explore-widget-${id}`)).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth), locale).toBe(0);
  }
});

test('a failed explore summary stays in its card', async ({ page }) => {
  await readyForLedger(page);
  await page.route('**/api/v1/home', async route => {
    const response = await route.fetch();
    const json = await response.json();
    json.widgets.backups = { status: 'failed', summary: null };
    await route.fulfill({ response, json });
  });
  await page.goto('/app');
  const backups = page.getByTestId('explore-widget-backups');
  await expect(backups).toHaveAttribute('data-state', 'failed');
  await expect(backups.getByRole('alert')).toHaveText('This summary could not be loaded.');
  await expect(page.getByTestId('explore-widget-prices')).not.toHaveAttribute('data-state', 'failed');
});

// Home shows a saved run for one account; Open returns opens that very run,
// not a live read of its period over another scope (#479 review).
test('Returns on Home opens the saved run it shows', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  await apiJSON(page, 'POST', `/api/v1/currencies/${currencyID}/default`, csrfToken, undefined, [200]);
  const opened = daysFromTodayISO(-60);
  const account = (name: string) => apiJSON<{ id: number }>(page, 'POST', '/api/v1/accounts', csrfToken, {
    name, account_class: 'asset', account_kind: 'checking', default_commodity_id: currencyID, allows_postings: true, opened_on: opened, effective_from: opened
  });
  const checking = await account('Explore run checking');
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: 'HOMERUN', instrument_type: 'stock', display_name: 'Home run example', symbol: 'HOMERUN',
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: opened
  });
  const holdings = [];
  for (const name of ['Explore run holding A', 'Explore run holding B']) {
    holdings.push(await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, { instrument_id: instrument.id, name, opened_on: opened, effective_from: opened }));
  }
  for (const holding of holdings) {
    await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
      transaction_date: daysFromTodayISO(-40), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
      cash_account_id: checking.id, quantity_value: '10', quantity_scale: 0, cash_amount_value: '100000', cash_amount_scale: 2,
      cash_commodity_id: currencyID, cost_basis_method: 'fifo'
    });
  }
  await apiJSON(page, 'POST', '/api/v1/pricing/prices', csrfToken, {
    base_commodity_id: instrument.commodity_id, quote_commodity_id: currencyID, quote_type: 'close',
    adjustment_basis: 'raw', price_value: 11000, price_scale: 2, valuation_date: todayISO(), is_manual: true
  });
  // A run over one of the two holdings only.
  const run = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/returns/runs', csrfToken, {
    from: daysFromTodayISO(-45), to: todayISO(), reporting_currency_id: currencyID, account_ids: [holdings[0].id]
  });

  await page.goto('/app');
  const card = page.getByTestId('explore-widget-returns');
  await expect(card).toHaveAttribute('data-state', 'ready');
  await expect(card.getByText('Saved run')).toBeVisible();
  const link = card.getByRole('link', { name: /Open returns/ });
  await expect(link).toHaveAttribute('href', `/app/investments/returns?run_id=${run.id}`);
  await expect(page.getByTestId('explore-widget-allocation').getByRole('link', { name: /Open returns/ })).toHaveAttribute('href', `/app/investments/returns?run_id=${run.id}`);
  await link.click();
  const heading = page.getByTestId('saved-returns-run').getByRole('heading', { name: new RegExp(`^Saved run #${run.id} · `) });
  await expect(heading).toBeFocused();
  await expect(page.getByTestId('returns-runs').getByRole('button', { name: new RegExp(`^#${run.id} · saved `) })).toHaveAttribute('aria-pressed', 'true');
});
