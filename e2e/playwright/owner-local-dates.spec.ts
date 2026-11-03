import { expect, test } from './support/fixtures';
import { readyForLedger } from './support/ledger';
import { apiJSON } from './support/api';

// At any UTC hour one of these owner zones is on a different day from UTC.
const ownerZone = new Date().getUTCHours() >= 10 ? 'Pacific/Kiritimati' : 'Pacific/Honolulu';
const browserZone = ownerZone === 'Pacific/Kiritimati' ? 'Pacific/Honolulu' : 'Pacific/Kiritimati';
test.use({ timezoneId: browserZone, viewport: { width: 390, height: 844 } });

function calendarDate(instant: string, zone: string) {
  const parts = new Intl.DateTimeFormat('en', { timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit' }).formatToParts(new Date(instant));
  const part = (type: string) => parts.find(p => p.type === type)!.value;
  return `${part('year')}-${part('month')}-${part('day')}`;
}

test('[acceptance] financial defaults use owner-local today across midnight and preserve chosen dates', async ({ page }) => {
  const { csrfToken } = await readyForLedger(page);
  const preferences = await apiJSON<{ updated_at: string }>(page, 'PUT', '/api/v1/settings/preferences', csrfToken, { time_zone: ownerZone });
  const today = calendarDate(preferences.updated_at, ownerZone);
  expect(today).not.toBe(calendarDate(preferences.updated_at, browserZone));
  expect(today).not.toBe(preferences.updated_at.slice(0, 10));
  await page.clock.setFixedTime(new Date(preferences.updated_at));
  try {
    await page.goto('/app/ledger/transactions');
    await page.getByRole('button', { name: 'Add transaction', exact: true }).click();
    const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Add transaction', exact: true }) });
    const date = form.getByLabel('Date', { exact: true });
    await expect(date).toHaveValue(today);
    const session = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
    expect(session.owner_today).toBe(today);
    await date.fill('2020-02-29');
    await form.getByLabel('Description', { exact: true }).fill('Keep this unsaved entry');
    // Change the owner zone, then trigger the real shared session query's focus refresh.
    const changed = await apiJSON<{ updated_at: string }>(page, 'PUT', '/api/v1/settings/preferences', csrfToken, { time_zone: 'UTC' });
    const refreshed = page.waitForResponse(response => new URL(response.url()).pathname === '/api/v1/auth/session');
    await page.evaluate(() => window.dispatchEvent(new Event('visibilitychange')));
    expect((await (await refreshed).json()).owner_today).toBe(changed.updated_at.slice(0, 10));
    await expect(date).toHaveValue('2020-02-29');
    await expect(form.getByLabel('Description', { exact: true })).toHaveValue('Keep this unsaved entry');
    await form.getByRole('button', { name: 'Cancel', exact: true }).first().click();
    await page.getByRole('button', { name: 'Add transaction', exact: true }).click();
    await expect(date).toHaveValue(changed.updated_at.slice(0, 10));
    await apiJSON(page, 'PUT', '/api/v1/settings/preferences', csrfToken, { time_zone: ownerZone });

    await page.goto('/app/investments');
    await page.getByRole('button', { name: 'Record buy', exact: true }).click();
    const buy = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Record buy', exact: true }) });
    await expect(buy.getByLabel('Date', { exact: true })).toHaveValue(today);

    await page.goto('/app/reports/net-worth');
    await expect(page).toHaveURL(new RegExp(`end_date=${today}`));
    expect(new URL(page.url()).searchParams.get('start_date')).toBe(`${today.slice(0, 4)}-01-01`);
    await page.goto('/app/reports/net-worth?start_date=2020-01-01&end_date=2020-02-29&bucket=day');
    await expect(page.getByLabel('End date', { exact: true })).toHaveValue('2020-02-29');

    await page.goto('/app/investments/prices');
    await page.getByRole('button', { name: 'Add manual observation', exact: true }).click();
    await expect(page.locator('#price-date')).toHaveValue(today);
    await page.locator('#price-date').fill('2020-02-29');
    await expect(page.locator('#price-date')).toHaveValue('2020-02-29');
  } finally {
    await apiJSON(page, 'PUT', '/api/v1/settings/preferences', csrfToken, { time_zone: 'UTC' });
  }
});
