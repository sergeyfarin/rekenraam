import { expect, test } from './support/fixtures';
import { ensureBrowserSession } from './support/session';
import { apiJSON } from './support/api';
import { expectNoAccessibilityViolations, expectFocusTrapped } from './support/a11y';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import en from '../../frontend/messages/settings/en.json' with { type: 'json' };
import es from '../../frontend/messages/settings/es.json' with { type: 'json' };
import fr from '../../frontend/messages/settings/fr.json' with { type: 'json' };
import nl from '../../frontend/messages/settings/nl.json' with { type: 'json' };
import de from '../../frontend/messages/settings/de.json' with { type: 'json' };
import ru from '../../frontend/messages/settings/ru.json' with { type: 'json' };

test.use({ viewport: { width: 390, height: 844 } });

for (const theme of ['light', 'dark']) {
  test(`[acceptance] personal access tokens show once, enforce scope and revoke on a ${theme} phone`, async ({ page, backend }) => {
    await ensureBrowserSession(page);
    await page.evaluate((theme) => localStorage.setItem('rekenraam-theme', theme), theme);
    await page.goto('/app/settings/security');
    const panel = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Personal access tokens', exact: true }) }).last();
    await expect(panel.getByRole('button', { name: 'Create token', exact: true })).toBeEnabled();
    const name = `Synthetic ${theme} phone client`;
    await panel.getByLabel('Token name', { exact: true }).fill(name);
    await panel.getByLabel('Confirm your password', { exact: true }).fill('test-password');
    await panel.getByRole('button', { name: 'Create token', exact: true }).click();
    const secretField = panel.getByLabel('Your new token', { exact: true });
    await expect(secretField).toBeVisible();
    await expect(secretField).toBeFocused();
    const secret = await secretField.inputValue();
    expect(secret).toMatch(/^rr_pat_[A-Za-z0-9_-]{43}$/);
    await expectNoAccessibilityViolations(page, `${theme} phone token creation`);
    const stored = await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, session: { ...sessionStorage } }));
    expect(stored).not.toContain(secret);

    const statuses = await page.evaluate(async (token) => {
      const headers = { Authorization: `Bearer ${token}` };
      const paths = ['/api/v1/accounts', '/api/v1/reports/cashflow', '/api/v1/auth/tokens', '/api/v1/settings/preferences'];
      const results = [];
      for (const path of paths) results.push((await fetch(path, { headers, credentials: 'omit' })).status);
      return results;
    }, secret);
    expect(statuses).toEqual([200, 403, 403, 403]);
    await panel.getByRole('button', { name: 'I have saved the token', exact: true }).click();
    await expect(secretField).toHaveCount(0);
    await expect(page.getByText(secret, { exact: true })).toHaveCount(0);
    await page.reload();
    await expect(page.getByLabel('Your new token', { exact: true })).toHaveCount(0);
    const metadata = await apiJSON<{ tokens: Array<{ name: string; id: number }> }>(page, 'GET', '/api/v1/auth/tokens');
    expect(JSON.stringify(metadata)).not.toContain(secret);
    const token = metadata.tokens.find((token) => token.name === name)!;
    expect(token).toBeDefined();
    const row = panel.locator('li').filter({ has: page.getByText(name, { exact: true }) });
    await row.getByRole('button', { name: 'Revoke token', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Revoke token', exact: true });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeFocused();
    await expectFocusTrapped(page, dialog, `${theme} phone token revocation`);
    await expectNoAccessibilityViolations(page, `${theme} phone token revocation`);
    await dialog.getByRole('button', { name: 'Revoke token', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect(row.getByText('Revoked', { exact: true })).toBeVisible();
    const revokedStatus = await page.evaluate(async (token) => (await fetch('/api/v1/accounts', {
      credentials: 'omit', headers: { Authorization: `Bearer ${token}` }
    })).status, secret);
    expect(revokedStatus).toBe(401);
    const events = await apiJSON<{ events: Array<{ event_type: string; metadata?: { token_id?: number } }> }>(page, 'GET', '/api/v1/auth/events');
    expect(events.events.some((event) => event.event_type === 'token_created' && event.metadata?.token_id === token.id)).toBe(true);
    expect(events.events.some((event) => event.event_type === 'token_revoked' && event.metadata?.token_id === token.id)).toBe(true);
    expect(JSON.stringify(events)).not.toContain(secret);
    if (backend.databasePath) {
      const log = readFileSync(join(dirname(backend.databasePath), 'app.log'), 'utf8');
      expect(log).toContain('/api/v1/auth/tokens');
      expect(log).not.toContain(secret);
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  });
}

test('[acceptance] token metadata loading and failure states recover without retaining passwords', async ({ page }) => {
  await ensureBrowserSession(page);
  let release!: () => void;
  const wait = new Promise<void>((resolve) => (release = resolve));
  await page.route('**/api/v1/auth/tokens', async (route) => {
    await wait;
    await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: { code: 'INTERNAL_ERROR', message: 'synthetic failure' } }) });
  });
  await page.goto('/app/settings/security');
  await expect(page.getByText('Loading tokens…', { exact: true })).toBeVisible();
  release();
  await expect(page.getByRole('button', { name: 'Try again', exact: true })).toBeVisible({ timeout: 15_000 });
  await page.unroute('**/api/v1/auth/tokens');
  await page.getByRole('button', { name: 'Try again', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Try again', exact: true })).toHaveCount(0);
  await page.getByLabel('Token name', { exact: true }).fill('Synthetic wrong-password client');
  await page.getByLabel('Confirm your password', { exact: true }).fill('wrong-password');
  await page.getByRole('button', { name: 'Create token', exact: true }).click();
  await expect(page.getByRole('alert')).toBeVisible();
  await expect(page.getByLabel('Confirm your password', { exact: true })).toHaveValue('');
  await expect(page.getByLabel('Your new token', { exact: true })).toHaveCount(0);
});

test('[acceptance] hiding a document scrubs its one-time token before browser history can retain it', async ({ page }) => {
  await ensureBrowserSession(page);
  await page.goto('/app/settings/security');
  await page.getByLabel('Token name', { exact: true }).fill('Synthetic pagehide client');
  await page.getByLabel('Confirm your password', { exact: true }).fill('test-password');
  await page.getByRole('button', { name: 'Create token', exact: true }).click();
  const secretField = page.getByLabel('Your new token', { exact: true });
  await expect(secretField).toBeVisible();
  const secret = await secretField.inputValue();
  const scrubbed = await page.evaluate(() => {
    const field = document.querySelector('textarea[readonly]') as HTMLTextAreaElement;
    window.dispatchEvent(new PageTransitionEvent('pagehide', { persisted: true }));
    return field.value;
  });
  expect(scrubbed).toBe('');
  await expect(secretField).toHaveCount(0);
  await expect(page.getByLabel('Confirm your password', { exact: true })).toHaveValue('');
  await page.reload();
  expect(await page.locator('body').innerText()).not.toContain(secret);
  const row = page.locator('li').filter({ has: page.getByText('Synthetic pagehide client', { exact: true }) });
  await row.getByRole('button', { name: 'Revoke token', exact: true }).click();
  await page.getByRole('dialog', { name: 'Revoke token', exact: true }).getByRole('button', { name: 'Revoke token', exact: true }).click();
  await expect(row.getByText('Revoked', { exact: true })).toBeVisible();
});

test('[acceptance] a delayed creation response cannot restore a token after document hiding', async ({ page }) => {
  await ensureBrowserSession(page);
  await page.goto('/app/settings/security');
  let arrived!: () => void;
  let release!: () => void;
  const responseArrived = new Promise<void>((resolve) => (arrived = resolve));
  const wait = new Promise<void>((resolve) => (release = resolve));
  await page.route('**/api/v1/auth/tokens', async (route) => {
    if (route.request().method() !== 'POST') return route.continue();
    const response = await route.fetch();
    arrived();
    await wait;
    await route.fulfill({ response });
  });
  await page.getByLabel('Token name', { exact: true }).fill('Synthetic delayed client');
  await page.getByLabel('Confirm your password', { exact: true }).fill('test-password');
  await page.getByRole('button', { name: 'Create token', exact: true }).click();
  await responseArrived;
  await page.evaluate(() => window.dispatchEvent(new PageTransitionEvent('pagehide', { persisted: true })));
  release();
  await expect(page.getByRole('button', { name: 'Create token', exact: true })).toBeEnabled();
  await expect(page.getByLabel('Your new token', { exact: true })).toHaveCount(0);
  await page.unroute('**/api/v1/auth/tokens');
  const row = page.locator('li').filter({ has: page.getByText('Synthetic delayed client', { exact: true }) });
  await row.getByRole('button', { name: 'Revoke token', exact: true }).click();
  await page.getByRole('dialog', { name: 'Revoke token', exact: true }).getByRole('button', { name: 'Revoke token', exact: true }).click();
  await expect(row.getByText('Revoked', { exact: true })).toBeVisible();
});

for (const [locale, messages] of Object.entries({ en, es, fr, nl, de, ru })) {
  test(`[acceptance] read-only token permissions render in ${locale} on a phone`, async ({ page }) => {
    await ensureBrowserSession(page);
    await page.evaluate((locale) => localStorage.setItem('PARAGLIDE_LOCALE', locale), locale);
    await page.goto('/app/settings/security');
    await expect(page.getByRole('heading', { name: messages.security_tokens_title, exact: true })).toBeVisible();
    await expect(page.getByLabel(messages.security_tokens_ledger, { exact: true })).toBeChecked();
    await expect(page.getByLabel(messages.security_tokens_reports, { exact: true })).not.toBeChecked();
    await expect(page.getByLabel(messages.security_tokens_days, { exact: true })).toHaveValue('30');
    await expect(page.getByRole('button', { name: messages.security_tokens_create, exact: true })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  });
}
