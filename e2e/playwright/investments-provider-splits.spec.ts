import { randomUUID } from 'node:crypto';
import process from 'node:process';
import { expect, test } from './support/fixtures';
import { expectNoAccessibilityViolations } from './support/a11y';
import { apiJSON } from './support/api';
import { daysFromTodayISO } from './support/dates';
import { createCashAccount, readyForLedger } from './support/ledger';

test.use({ viewport: { width: 390, height: 844 } });
test.skip(Boolean(process.env.E2E_BASE_URL), 'needs the local Massive stub');

test('[acceptance] a Massive split is reviewed and posted once on a phone', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const reference = new Date();
  const openedOn = daysFromTodayISO(-60, reference);
  const splitDate = daysFromTodayISO(-10, reference);
  const symbol = `E2ESPLIT${randomUUID().replaceAll('-', '').slice(0, 12).toUpperCase()}`;
  const name = `Split issuer ${symbol}`;
  const stubURL = process.env.E2E_TRADING212_STUB_URL ?? `http://127.0.0.1:${process.env.TRADING212_STUB_PORT ?? '16890'}`;
  const registered = await fetch(`${stubURL}/__control/splits/${symbol}`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ execution_date: splitDate })
  });
  expect(registered.status).toBe(204);

  const cash = await createCashAccount(page, csrfToken, `Cash ${symbol}`, currencyID, openedOn);
  const instrument = await apiJSON<{ id: number; commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: symbol, instrument_type: 'stock', display_name: name, symbol, mic: 'XNAS',
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: openedOn
  });
  const holding = await apiJSON<{ id: number }>(page, 'POST', '/api/v1/investments/holding-accounts', csrfToken, {
    instrument_id: instrument.id, name: `Holding ${symbol}`, opened_on: openedOn, effective_from: openedOn
  });
  await apiJSON(page, 'POST', '/api/v1/investments/buy', csrfToken, {
    transaction_date: daysFromTodayISO(-30, reference), commodity_id: instrument.commodity_id, holding_account_id: holding.id,
    cash_account_id: cash.id, cash_commodity_id: currencyID, quantity_value: '10', quantity_scale: 0,
    cash_amount_value: '100000', cash_amount_scale: 2
  });

  await page.goto('/app/investments/instruments');
  await page.getByRole('button', { name: `${name} (${symbol})`, exact: true }).click();
  await page.getByLabel('Massive ticker').fill(symbol);
  await page.getByRole('button', { name: 'Save instrument', exact: true }).click();
  await expect(page.getByText('Saved.', { exact: true })).toBeVisible();

  await page.goto('/app/investments');
  await page.getByRole('link', { name: /^Suggestions/ }).click();
  await page.getByRole('button', { name: 'Check Massive splits', exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Split check queued for 1 listing.' })).toBeVisible();
  const card = page.getByTestId('event-suggestion').filter({ hasText: name });
  await expect(card).toContainText('4 new for 1 old', { timeout: 30_000 });
  await expect(card.locator('time')).toHaveAttribute('datetime', splitDate);
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  await expectNoAccessibilityViolations(page, 'Massive split suggestion on a phone');

  await card.getByRole('button', { name: 'Accept', exact: true }).click();
  const settled = page.getByRole('listitem').filter({ hasText: name }).filter({ hasText: 'Accepted' });
  await expect(settled).toContainText('4 new for 1 old');
  await expect(settled.getByRole('link', { name: /^Open split #/ })).toBeVisible();
  await page.getByRole('button', { name: 'Check Massive splits', exact: true }).click();
  await expect(settled).toHaveCount(1);
  await expect(card).toHaveCount(0);
});

test('split cards offer manual review or a link without showing Accept', async ({ page }) => {
  await readyForLedger(page);
  const split = {
    book_id: 1, event_family: 'split', event_date: '2026-06-16', event_status: 'new',
    status: 'suggested', source_code: 'massive', confidence_bps: 10000,
    evidence: { schema: 1, ratio_numerator: 4, ratio_denominator: 1 }, link_candidates: [],
    revisions: [], created_at: '2026-06-16T12:00:00Z', updated_at: '2026-06-16T12:00:00Z'
  };
  const revision = { observed_at: '2026-06-17T12:00:00Z', evidence: { schema: 1, ratio_numerator: 5, ratio_denominator: 1 } };
  await page.route('**/api/v1/investments/event-suggestions', (route) => route.fulfill({ json: { suggestions: [
    { ...split, id: 1, provider_event_id: 1, confidence_bps: 0, proposed_transaction: { kind: 'split', review_reason: 'identity' },
      evidence: { schema: 1, ratio_numerator: 1, ratio_denominator: 20 } },
    { ...split, id: 2, provider_event_id: 2, proposed_transaction: { kind: 'split', holding_account_id: 42 },
      split_link_candidates: [{ transaction_id: 12, holding_account_id: 42, holding_account_name: 'Broker holding' }] },
    { ...split, id: 3, provider_event_id: 3, proposed_transaction: { kind: 'split', holding_account_id: 42 }, revisions: [revision] },
    { ...split, id: 4, provider_event_id: 4, status: 'linked', event_status: 'matched',
      linked_transaction_id: 13, proposed_transaction: { kind: 'split', holding_account_id: 42 }, revisions: [revision] }
  ] } }));
  await page.goto('/app/investments');
  await page.getByRole('link', { name: /^Suggestions/ }).click();
  const cards = page.getByTestId('event-suggestion');
  await expect(cards).toHaveCount(3);
  await expect(cards.nth(0)).toContainText('1 new for 20 old');
  await expect(cards.nth(0)).toContainText('Massive could not verify the listing on the split date');
  await expect(cards.nth(1).getByRole('button', { name: 'Link recorded split' })).toBeVisible();
  await expect(cards.getByRole('button', { name: 'Accept', exact: true })).toHaveCount(0);
  await expect(page.getByTestId('split-suggestion-revisions')).toHaveCount(2);
  await expect(page.getByTestId('event-suggestion-revisions')).toHaveCount(0);
});
