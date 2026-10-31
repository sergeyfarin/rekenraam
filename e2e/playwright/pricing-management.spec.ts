import { test, expect, type Page } from './support/fixtures';
import AxeBuilder from '@axe-core/playwright';
import { readFileSync } from 'node:fs';
import { readyForLedger, ensureCurrency } from './support/ledger';
import { apiJSON } from './support/api';
import { daysFromTodayISO, todayISO } from './support/dates';
import { useDarkTheme } from './support/session';

test.use({ viewport: { width: 390, height: 844 } });
const messages = (locale = 'en') => JSON.parse(readFileSync(new URL(`../../frontend/messages/pricing/${locale}.json`, import.meta.url), 'utf8'));
async function setup(page: Page, locale = 'en') {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const eur = await ensureCurrency(page, csrfToken, 'EUR', 'Euro');
  const usd = await ensureCurrency(page, csrfToken, 'USD', 'US Dollar');
  const base = currencyID === eur ? usd : eur;
  await page.evaluate(value => localStorage.setItem('PARAGLIDE_LOCALE', value), locale);
  await page.goto(`/app/investments/prices?base_commodity_id=${base}&quote_commodity_id=${currencyID}`);
  await expect(page.locator('#pricing-history-base')).toHaveValue(String(base));
  return { csrfToken, base, quote: currencyID };
}

for (const locale of ['en', 'es', 'fr', 'nl', 'de', 'ru']) {
  test(`[acceptance] exact manual entry, correction and withdrawal at 390px in ${locale}`, async ({ page }) => {
    const s = await setup(page, locale);
    const editable = (value: string) => value.replace('.', locale === 'en' ? '.' : ',');
    const m = messages(locale);
    if (locale === 'en') await useDarkTheme(page);
    const add = page.getByRole('button', { name: m.pricing_add, exact: true });
    await add.focus(); await page.keyboard.press('Enter');
    const dialog = page.getByRole('dialog', { name: m.pricing_add, exact: true });
    await expect(dialog.locator('#price-base')).toBeFocused();
    await dialog.locator('#price-amount').fill(editable('0.009007199254740993'));
    await dialog.locator('#price-quantity').fill(editable('0.005'));
    await dialog.locator('#price-reference').fill(`Synthetic statement ${locale}`);
    const review = dialog.getByRole('button', { name: m.pricing_review, exact: true });
    await expect(review).toBeDisabled();
    await dialog.locator('#price-reason').fill('Manual statement valuation');
    await review.focus(); await page.keyboard.press('Enter');
    const region = dialog.getByRole('region', { name: m.pricing_review_title });
    await expect(region).toBeVisible();
    expect((await new AxeBuilder({ page }).include('dialog').analyze()).violations).toEqual([]);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    // Cancel the review with Escape: no write, and input remains available.
    const before = await apiJSON<{prices: unknown[]}>(page, 'GET', `/api/v1/pricing/history?base_commodity_id=${s.base}&quote_commodity_id=${s.quote}&include_voided=true&limit=100`);
    await page.keyboard.press('Escape');
    await expect(dialog.locator('#price-amount')).toHaveValue(editable('0.009007199254740993'));
    expect((await apiJSON<{prices: unknown[]}>(page, 'GET', `/api/v1/pricing/history?base_commodity_id=${s.base}&quote_commodity_id=${s.quote}&include_voided=true&limit=100`)).prices.length).toBe(before.prices.length);
    await review.click();
    const createdResponse = page.waitForResponse(res => res.url().endsWith('/pricing/manual') && res.request().method() === 'POST');
    await dialog.getByRole('button', { name: m.pricing_confirm, exact: true }).click();
    const response = await createdResponse; expect(response.status()).toBe(201);
    expect(response.request().postDataJSON().price_value).toBe('9007199254740993');
    const created = await response.json(); expect(created.price_value).toBe('9007199254740993');
    await expect(dialog).toBeHidden(); await expect(add).toBeFocused();
    await expect(page.getByRole('status').filter({ hasText: m.pricing_saved })).toBeVisible();
    const row = page.locator(`[data-price-id="${created.id}"]`);
    await row.getByRole('button', { name: m.pricing_correct, exact: true }).click();
    const correct = page.getByRole('dialog', { name: m.pricing_correct, exact: true });
    await expect(correct.locator('#price-amount')).toHaveValue(editable('0.009007199254740993'));
    await expect(correct.locator('#price-date')).toBeDisabled();
    await correct.locator('#price-amount').fill(editable('0.009007199254740995'));
    await correct.locator('#price-reason').fill('Correct synthetic statement');
    await correct.getByRole('button', { name: m.pricing_review, exact: true }).click();
    await expect(correct).toContainText(m.pricing_retire_list);
    const replacementResponse = page.waitForResponse(res => res.url().endsWith(`/prices/${created.id}/correct`) && res.request().method() === 'POST');
    await correct.getByRole('button', { name: m.pricing_confirm, exact: true }).click();
    const replacement = await (await replacementResponse).json();
    expect(replacement.replacement.price_value).toBe('9007199254740995');
    await expect(correct).toBeHidden();
    await expect(row).toContainText(m.pricing_retired);
    const current = page.locator(`[data-price-id="${replacement.replacement.id}"]`);
    await current.getByRole('button', { name: m.pricing_void, exact: true }).click();
    const withdraw = page.getByRole('dialog', { name: m.pricing_void, exact: true });
    await withdraw.locator('#price-reason').fill('Withdraw synthetic assertion');
    await withdraw.getByRole('button', { name: m.pricing_review, exact: true }).click();
    await withdraw.getByRole('button', { name: m.pricing_confirm, exact: true }).click();
    await expect(withdraw).toBeHidden(); await expect(current).toContainText(m.pricing_retired);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
  });
}

test('[acceptance] new derived dependency refuses stale correction and retains input until re-reviewed', async ({ page }) => {
  const s = await setup(page); const m = messages();
  const created = await apiJSON<{id:number}>(page, 'POST', '/api/v1/pricing/manual', s.csrfToken, {
    base_commodity_id:s.base,quote_commodity_id:s.quote,valuation_date:todayISO(),price_value:'100',price_scale:2,base_quantity_value:'1',base_quantity_scale:0,reason:'Stale fixture'
  });
  await page.getByRole('button', { name:m.pricing_refresh, exact:true }).first().click();
  await page.locator(`[data-price-id="${created.id}"]`).getByRole('button',{name:m.pricing_correct,exact:true}).click();
  const dialog=page.getByRole('dialog',{name:m.pricing_correct,exact:true});
  await dialog.locator('#price-amount').fill('1.25');await dialog.locator('#price-reason').fill('Correct with fresh closure');
  await dialog.getByRole('button',{name:m.pricing_review,exact:true}).click();
  const derived=await apiJSON<{id:number}>(page,'POST','/api/v1/pricing/prices',s.csrfToken,{
    base_commodity_id:s.base,quote_commodity_id:s.quote,valuation_date:todayISO(),price_value:110,price_scale:2,base_quantity_value:1,base_quantity_scale:0,
    is_derived:true,derivation:{legs:[{observation_id:created.id}]},metadata:{},series_metadata:{}
  });
  await dialog.getByRole('button',{name:m.pricing_confirm,exact:true}).click();
  await expect(dialog.getByRole('alert')).toContainText(m.pricing_stale_review);
  await expect(dialog.locator('#price-amount')).toHaveValue('1.25');
  await expect(dialog.getByRole('region',{name:m.pricing_review_title})).toBeHidden();
  await dialog.getByRole('button',{name:m.pricing_review,exact:true}).click();
  await expect(dialog).toContainText(new Intl.NumberFormat('en').format(derived.id));
  await dialog.getByRole('button',{name:m.pricing_confirm,exact:true}).click();await expect(dialog).toBeHidden();
  const history=await apiJSON<{prices:Array<{id:number;voided_at?:string;supersedes_observation_id?:number}>}>(page,'GET',`/api/v1/pricing/history?base_commodity_id=${s.base}&quote_commodity_id=${s.quote}&include_voided=true&limit=100`);
  expect(history.prices.find(p=>p.id===derived.id)?.voided_at).toBeTruthy();
  expect(history.prices.find(p=>p.supersedes_observation_id===created.id)).toBeTruthy();
});

test('[acceptance] workspace loading, recoverable error and empty history remain accessible', async ({ page }) => {
  await readyForLedger(page); const m=messages();
  let release!:()=>void;const wait=new Promise<void>(resolve=>{release=resolve;});
  await page.route('**/api/v1/pricing/workspace*',async route=>{await wait;await route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:{code:'RESOURCE_BUSY',message:'fixture'}})});},{times:2});
  await page.goto('/app/investments/prices');await expect(page.getByText(m.pricing_loading,{exact:true})).toBeVisible();release();
  await expect(page.getByText(m.pricing_error,{exact:true})).toBeVisible();
  await page.getByRole('button',{name:m.pricing_refresh,exact:true}).first().click();
  await expect(page.locator('#pricing-history-base')).toBeVisible();
  await expect(page.getByText(m.pricing_choose_pair,{exact:true})).toBeVisible();
  expect((await new AxeBuilder({page}).include('main').analyze()).violations).toEqual([]);
});

test('[acceptance] empty configured instrument and older history pages are reachable without row requests', async ({ page }) => {
  const s=await setup(page); const m=messages();
  const name=`PAGE${Date.now()}`;
  const instrument=await apiJSON<{commodity_id:number}>(page,'POST','/api/v1/investments/instruments',s.csrfToken,{
    commodity_code:name,instrument_type:'stock',display_name:name,symbol:name,quote_commodity_id:s.quote,trading_commodity_id:s.quote,quantity_scale:0,price_scale:2
  });
  await page.goto(`/app/investments/prices?base_commodity_id=${instrument.commodity_id}&quote_commodity_id=${s.quote}`);
  await expect(page.getByText(m.pricing_history_empty,{exact:true})).toBeVisible();
  const missingCard=page.locator('li').filter({has:page.getByRole('button',{name:m.pricing_pair.replace('{base}',name).replace('{quote}',(await apiJSON<{currencies:Array<{id:number;code:string}>}>(page,'GET','/api/v1/currencies')).currencies.find(c=>c.id===s.quote)!.code),exact:true})});
  await expect(missingCard).toContainText(m.pricing_missing);
  const ids:number[]=[];
  for(let i=0;i<53;i++){
    const p=await apiJSON<{id:number}>(page,'POST','/api/v1/pricing/manual',s.csrfToken,{
      base_commodity_id:instrument.commodity_id,quote_commodity_id:s.quote,valuation_date:todayISO(),price_value:String(100+i),price_scale:2,base_quantity_value:'1',base_quantity_scale:0,reason:'Pagination fixture'
    });ids.push(p.id);
  }
  let historyRequests=0;
  page.on('request',request=>{if(request.url().includes('/api/v1/pricing/history'))historyRequests++;});
  await page.getByRole('button',{name:m.pricing_refresh,exact:true}).first().click();
  await expect(page.locator('[data-price-id]')).toHaveCount(50);
  await expect(page.locator(`[data-price-id="${ids[0]}"]`)).toHaveCount(0);
  await page.getByRole('button',{name:m.pricing_load_more,exact:true}).click();
  await expect(page.locator('[data-price-id]')).toHaveCount(53);
  await expect(page.locator(`[data-price-id="${ids[0]}"]`)).toBeVisible();
  await expect(page.getByRole('button',{name:m.pricing_load_more,exact:true})).toBeHidden();
  expect(historyRequests).toBe(2);
  expect(await page.evaluate(()=>document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
});

// R13 slice 7 (#119): a total-return benchmark series is entered in the
// browser. It is labelled benchmark-only, a correction keeps its basis, and
// the returns screen reads it as a total-return benchmark.
test('[acceptance] a manual total-return price feeds a total-return benchmark at 390px', async ({ page }) => {
  const { csrfToken, currencyID } = await readyForLedger(page);
  const m = messages();
  const opened = daysFromTodayISO(-60);
  // The returns screen needs an investment account to compare against.
  await apiJSON(page, 'POST', '/api/v1/accounts', csrfToken, {
    name: 'Total-return broker', account_class: 'asset', account_kind: 'brokerage_cash',
    default_commodity_id: currencyID, allows_postings: true, opened_on: opened, effective_from: opened
  });
  const index = await apiJSON<{ commodity_id: number }>(page, 'POST', '/api/v1/investments/instruments', csrfToken, {
    commodity_code: 'TRIDX', instrument_type: 'other', display_name: 'Total-return index', symbol: 'TRIDX',
    quote_commodity_id: currencyID, trading_commodity_id: currencyID, quantity_scale: 0, price_scale: 2, effective_from: opened
  });
  await page.goto(`/app/investments/prices?base_commodity_id=${index.commodity_id}&quote_commodity_id=${currencyID}`);
  await expect(page.locator('#pricing-history-base')).toHaveValue(String(index.commodity_id));
  const record = async (date: string, amount: string) => {
    await page.getByRole('button', { name: m.pricing_add, exact: true }).click();
    const dialog = page.getByRole('dialog', { name: m.pricing_add, exact: true });
    const basis = dialog.getByRole('combobox', { name: m.pricing_adjustment_basis, exact: true });
    await expect(basis).toHaveValue('raw');
    await expect(dialog.getByText(m.pricing_basis_hint, { exact: true })).toBeVisible();
    await basis.selectOption('total_return');
    await dialog.locator('#price-date').fill(date);
    await dialog.locator('#price-amount').fill(amount);
    await dialog.locator('#price-reason').fill('Index factsheet');
    await dialog.getByRole('button', { name: m.pricing_review, exact: true }).click();
    await expect(dialog.getByRole('region', { name: m.pricing_review_title })).toContainText(`${m.pricing_adjustment_basis}: ${m.pricing_basis_total}`);
    const saved = page.waitForResponse(res => res.url().endsWith('/pricing/manual') && res.request().method() === 'POST');
    await dialog.getByRole('button', { name: m.pricing_confirm, exact: true }).click();
    const response = await saved; expect(response.status()).toBe(201);
    expect(response.request().postDataJSON().adjustment_basis).toBe('total_return');
    await expect(dialog).toBeHidden();
    return (await response.json()) as { id: number };
  };
  const opening = await record(daysFromTodayISO(-46), '90');
  await record(todayISO(), '115');
  const row = page.locator(`[data-price-id="${opening.id}"]`);
  await expect(row).toContainText(`${m.pricing_basis_total} · ${m.pricing_benchmark_only}`);

  await row.getByRole('button', { name: m.pricing_correct, exact: true }).click();
  const correct = page.getByRole('dialog', { name: m.pricing_correct, exact: true });
  await expect(correct.getByRole('combobox', { name: m.pricing_adjustment_basis })).toHaveCount(0);
  await expect(correct).toContainText(m.pricing_basis_kept);
  await correct.locator('#price-amount').fill('100');
  await correct.locator('#price-reason').fill('Factsheet restated');
  await correct.getByRole('button', { name: m.pricing_review, exact: true }).click();
  const corrected = page.waitForResponse(res => res.url().endsWith(`/prices/${opening.id}/correct`) && res.request().method() === 'POST');
  await correct.getByRole('button', { name: m.pricing_confirm, exact: true }).click();
  const replacement = await (await corrected).json();
  expect(replacement.replacement.adjustment_basis).toBe('total_return');
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);

  const params = new URLSearchParams({ from: daysFromTodayISO(-45), to: todayISO(), reporting_currency_id: String(currencyID),
    benchmark_commodity_id: String(index.commodity_id), benchmark_basis: 'total_return' });
  await page.goto(`/app/investments/returns?${params}`);
  await expect(page.getByTestId('returns-benchmark').getByTestId('returns-benchmark-period')).toHaveText('+15.00%');
});
