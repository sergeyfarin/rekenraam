import { readFileSync } from 'node:fs';
import { expect, type Page } from '@playwright/test';

const renamed: Record<string, string> = {
  "Record buy": "Buy",
  "Record sell": "Sell",
  "Record write-off": "Write-off",
  "Record short sale": "Short sale",
  "Record short cover": "Short cover",
  "Record payment in lieu": "Payment in lieu",
  "Record dividend": "Cash dividend",
  "Record withholding tax": "Withholding tax",
  "Record reinvested dividend": "Reinvested dividend",
  "Transfer in investments": "Shares in",
  "Move between holdings": "Move shares between holdings",
  "Transfer out investments": "Shares out",
  "Record split": "Split",
  "Record share exchange": "Share exchange",
  "Record cash-and-stock merger": "Cash-and-stock merger",
  "Record spin-off": "Spin-off",
  "Return of capital": "Return of capital"
};

export async function openInvestmentActivity(page: Page, label: string): Promise<void> {
  const locale = await page.evaluate(() => localStorage.getItem('PARAGLIDE_LOCALE') ?? 'en');
  const english = JSON.parse(readFileSync(new URL('../../../frontend/messages/settings/en.json', import.meta.url), 'utf8')) as Record<string, string>;
  const messages = { ...english, ...JSON.parse(readFileSync(new URL(`../../../frontend/messages/settings/${locale}.json`, import.meta.url), 'utf8')) } as Record<string, string>;
  const selector = page.locator('#investment-activity');
  if (!(await selector.isVisible())) {
    await page.getByRole('button', { name: messages.transactions_form_create_title, exact: true }).click();
    await page.getByRole('tab', { name: messages.transactions_entry_tab_investment, exact: true }).click();
  }
  await expect(selector).toBeVisible();
  const ids = ['buy', 'sell', 'write-off', 'short-open', 'short-cover', 'payment-in-lieu', 'dividend', 'withholding', 'reinvested', 'external-transfer-in', 'internal-transfer', 'external-transfer-out', 'split', 'share-exchange', 'cash-and-stock-merger', 'spin-off', 'capital-return'];
  if (ids.includes(label)) await selector.selectOption(label);
  else await selector.selectOption({ label: renamed[label] ?? label });
  await page.getByRole('button', { name: messages.investments_activity_continue, exact: true }).click();
}
