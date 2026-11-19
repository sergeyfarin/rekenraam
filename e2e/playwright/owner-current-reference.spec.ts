import { test, expect, realUTCToday, utcDay } from './support/reference-date';
import { readyForLedger, createCashAccount } from './support/ledger';
import { apiJSON } from './support/api';

// Both owner days sit a week past SQLite's real UTC day, so an account opened
// on the owner's day is future-dated to any read the server forgot to pin.
const today = realUTCToday();
for (const control of [
  { name: 'east just after local midnight', instant: `${utcDay(today, 7)}T23:01:00Z`, zone: 'Europe/Amsterdam', today: utcDay(today, 8) },
  { name: 'west while UTC is tomorrow', instant: `${utcDay(today, 8)}T03:00:00Z`, zone: 'America/Los_Angeles', today: utcDay(today, 7) }
]) {
  test.describe(control.name, () => {
    test.use({ referenceInstant: control.instant, timezoneId: control.zone, viewport: { width: 390, height: 844 } });
    test('[acceptance] owner-today account is immediately fetched, listed and selectable', async ({ page }) => {
      const { csrfToken, currencyID } = await readyForLedger(page);
      await apiJSON(page, 'PUT', '/api/v1/settings/preferences', csrfToken, { time_zone: control.zone });
      const session = await apiJSON<{ owner_today: string }>(page, 'GET', '/api/v1/auth/session');
      expect(session.owner_today).toBe(control.today);
      await page.clock.setFixedTime(new Date(control.instant));
      const account = await createCashAccount(page, csrfToken, 'Owner today reference', currencyID);
      const fetched = await apiJSON<{ id: number; opened_on: string; effective_from: string }>(page, 'GET', `/api/v1/accounts/${account.id}`);
      expect(fetched).toMatchObject({ id: account.id, opened_on: control.today, effective_from: control.today });
      const listed = await apiJSON<{ accounts: Array<{ id: number }> }>(page, 'GET', '/api/v1/accounts?q=Owner%20today%20reference');
      expect(listed.accounts.map(item => item.id)).toContain(account.id);
      await page.goto('/app/ledger/transactions');
      await page.getByRole('button', { name: 'New transaction', exact: true }).click();
      const form = page.getByRole('region', { name: 'New transaction', exact: true });
      await expect(form.getByLabel('Date', { exact: true })).toHaveValue(control.today);
      const picker = form.getByRole('combobox', { name: 'Account', exact: true });
      await picker.selectOption({ label: account.name });
      await expect(picker).toHaveValue(String(account.id));
    });
  });
}
