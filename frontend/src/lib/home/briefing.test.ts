import { afterEach, describe, expect, it } from 'vitest';
import type { AttentionSource } from '#lib/api/attention.ts';
import type { Home, HomeHeadline } from '#lib/api/home.ts';
import type { AttentionState } from '#lib/attention/model.ts';
import { amountPrivacy } from '#lib/money/privacy.svelte.ts';
import { briefingHeadline, briefingStatus, headlineOptions, shownHeadlineID } from './briefing.ts';

const euro = (value: string) => ({ commodity_id: 1, quantity_value: value, quantity_scale: 2, normal_quantity_value: value });

function headline(id: HomeHeadline['id'], overrides: Partial<HomeHeadline> = {}): HomeHeadline {
  return { id, account_count: id === 'cash' ? 3 : 5, value: euro('5675040'), start_value: euro('5461040'), change: euro('214000'), unavailable: null, missing: [], stale: [], ...overrides };
}

function home(netWorth: Partial<HomeHeadline> = {}, cash: Partial<HomeHeadline> = {}): Home {
  return {
    as_of: '2026-10-15', start_boundary: '2026-09-30', empty_book: false,
    reporting_currency: { commodity_id: 1, code: 'EUR', kind: 'currency', symbol: '€', scale: 2 },
    headlines: [headline('net_worth', netWorth), headline('cash', cash)],
    // The briefing reads no widget; widgets.test.ts covers them.
    widgets: {
      spending: { status: 'failed', summary: null }, budgets: { status: 'failed', summary: null }, accounts: { status: 'failed', summary: null },
      recurring: { status: 'failed', summary: null }, net_worth_trend: { status: 'failed', summary: null }
    },
    commodities: []
  };
}

function source(id: AttentionSource['id'], count: number, overrides: Partial<AttentionSource> = {}): AttentionSource {
  return { id, unit: 'transaction', state: 'ready', status: '', count, failed: false, ...overrides };
}

function attention(sources: AttentionSource[], overrides: Partial<AttentionState> = {}): AttentionState {
  const total = sources.reduce((sum, item) => sum + (item.count ?? 0), 0);
  return {
    isPending: false, isError: false, isFetching: false, refetch: () => {},
    data: { checked_at: '2026-10-15T08:00:00Z', total_known: total, complete: sources.every(item => item.state === 'ready'), has_failures: sources.some(item => item.failed || item.state === 'error'), sources },
    ...overrides
  };
}

const missingPrices = (count: number) => Array.from({ length: count }, (_, index) => ({ commodity_id: 10 + index, code: `SEC${index}`, kind: 'price' as const }));

afterEach(() => { amountPrivacy.hidden = false; });

describe('headline', () => {
  it('shows net worth rounded for Home, with a signed month change and both boundaries', () => {
    const result = briefingHeadline(home(), 'net_worth', 'en')!;
    expect(result.label).toBe('Net worth');
    expect(result.amount).toBe('€56,750');
    expect(result.change).toEqual({ text: '+€2,140 in Oct', sign: 1, label: '+€2,140 from the end of Sep 30, 2026 to Oct 15, 2026' });
  });

  it('colours a fall by sign and keeps minor units below 100', () => {
    const result = briefingHeadline(home({ change: euro('-9640') }), 'net_worth', 'en')!;
    expect(result.change?.text).toBe('−€96.40 in Oct');
    expect(result.change?.sign).toBe(-1);
  });

  it('masks amounts and drops the colour while amounts are hidden', () => {
    amountPrivacy.hidden = true;
    const result = briefingHeadline(home(), 'net_worth', 'en')!;
    expect(result.amount).toBe('••••');
    expect(result.change?.text).toBe('•••• in Oct');
    expect(result.change?.sign).toBe(0);
  });

  it('falls back to cash while net worth is unavailable, never a partial figure', () => {
    const book = home({ value: null, change: null, start_value: null, unavailable: 'valuation_gaps', missing: missingPrices(3) });
    expect(shownHeadlineID(book, 'auto')).toBe('cash');
    expect(shownHeadlineID(book, 'net_worth')).toBe('net_worth');
    expect(briefingHeadline(book, 'net_worth', 'en')?.amount).toBeNull();
  });

  it('shows only the status line when cash also lacks a rate', () => {
    const book = home({ value: null, unavailable: 'valuation_gaps' }, { value: null, unavailable: 'valuation_gaps' });
    expect(shownHeadlineID(book, 'auto')).toBeNull();
  });

  it('states each choice scope as a count, never an account list', () => {
    expect(headlineOptions(home(), 'en').map(option => option.scope)).toEqual(['All 5 accounts', '3 cash accounts']);
    expect(headlineOptions(home({ account_count: 1 }, { account_count: 0, value: null, unavailable: 'no_accounts' }), 'en').map(option => option.scope)).toEqual(['1 account']);
  });

  it('keeps net worth a choice when every account is closed', () => {
    const book = home({ account_count: 0 }, { account_count: 0, value: null, unavailable: 'no_accounts' });
    expect(shownHeadlineID(book, 'auto')).toBe('net_worth');
    expect(headlineOptions(book, 'en')).toEqual([{ id: 'net_worth', label: 'Net worth', scope: 'Closed accounts only' }]);
  });
});

describe('status line', () => {
  it('says all clear only after a complete, successful check', () => {
    expect(briefingStatus(home(), 'net_worth', 'auto', attention([source('drafts', 0)]), 'en').lead.text).toBe('All clear, nothing to review');
    expect(briefingStatus(home(), 'net_worth', 'auto', attention([source('drafts', 0, { state: 'loading', count: null })]), 'en').lead.text).toBe('Checking what needs review…');
    expect(briefingStatus(home(), 'net_worth', 'auto', attention([], { data: undefined, isPending: true }), 'en').lead.text).toBe('Checking what needs review…');
  });

  it('names the work, two kinds at most', () => {
    const status = briefingStatus(home(), 'net_worth', 'auto', attention([source('drafts', 2), source('imports', 1, { unit: 'batch' })]), 'en');
    expect(status.lead).toEqual({ text: '2 drafts and 1 import to review', href: '/app/review' });
    expect(status.more).toBeNull();
    const three = briefingStatus(home(), 'net_worth', 'auto', attention([source('drafts', 2), source('imports', 1), source('suggestions', 4)]), 'en');
    expect(three.lead.text).toBe('2 drafts to review');
    expect(three.more).toEqual({ text: '5 more', href: '/app/review', label: '5 more to review' });
  });

  it('leads with the valuation problem behind the headline and counts the rest', () => {
    const book = home({ value: null, unavailable: 'valuation_gaps', missing: missingPrices(3) });
    const status = briefingStatus(book, 'cash', 'auto', attention([source('drafts', 2), source('prices', 3, { unit: 'pair' })]), 'en');
    expect(status.lead).toEqual({ text: 'Net worth needs 3 prices', href: '/app/investments/prices?status=unusable' });
    expect(status.more?.text).toBe('2 more');
  });

  it('names one missing input and counts mixed kinds as values', () => {
    const one = home({ value: null, unavailable: 'valuation_gaps', missing: [{ commodity_id: 7, code: 'VWRL', kind: 'price' }] });
    expect(briefingStatus(one, 'cash', 'auto', attention([]), 'en').lead.text).toBe('Net worth needs a price for VWRL');
    const rate = home({ value: null, unavailable: 'valuation_gaps', missing: [{ commodity_id: 8, code: 'USD', kind: 'rate' }] });
    expect(briefingStatus(rate, 'cash', 'auto', attention([]), 'en').lead).toEqual({ text: 'Net worth needs an exchange rate for USD', href: '/app/settings/currencies' });
    const mixed = home({ value: null, unavailable: 'valuation_gaps', missing: [...missingPrices(3), { commodity_id: 8, code: 'USD', kind: 'rate' }] });
    expect(briefingStatus(mixed, 'cash', 'auto', attention([]), 'en').lead.text).toBe('Net worth needs 4 values');
  });

  it('never blames a chosen cash headline for missing prices', () => {
    const book = home({ value: null, unavailable: 'valuation_gaps', missing: missingPrices(3) });
    expect(briefingStatus(book, 'cash', 'cash', attention([]), 'en').lead.text).toBe('3 prices missing');
  });

  it('counts out-of-date prices when nothing is missing', () => {
    const book = home({ stale: missingPrices(180) });
    expect(briefingStatus(book, 'net_worth', 'auto', attention([]), 'en').lead).toEqual({ text: '180 prices out of date', href: '/app/investments/prices?status=stale' });
  });

  it('lets failures lead and never shows all clear while one stands', () => {
    const book = home({ value: null, unavailable: 'valuation_gaps', missing: missingPrices(3) });
    const failed = briefingStatus(book, 'cash', 'auto', attention([source('drafts', 2), source('backups', 1, { unit: 'latest_run', status: 'failed', failed: true })]), 'en');
    expect(failed.tone).toBe('failed');
    expect(failed.lead.text).toBe('Latest backup failed');
    expect(failed.more?.text).toBe('2 more');
    const unchecked = briefingStatus(home(), 'net_worth', 'auto', attention([source('drafts', 0), source('imports', 0, { state: 'error', count: null, error_code: 'INTERNAL_ERROR' })]), 'en');
    expect(unchecked.lead.text).toBe('Some checks could not run');
    expect(briefingStatus(home(), 'net_worth', 'auto', attention([], { data: undefined, isError: true }), 'en').lead.text).toBe('Could not check what needs review');
  });

  it('never keeps an earlier all clear after a refresh fails', () => {
    const kept = attention([source('drafts', 0)], { isError: true });
    expect(kept.data?.total_known).toBe(0);
    const status = briefingStatus(home(), 'net_worth', 'auto', kept, 'en');
    expect(status.tone).toBe('failed');
    expect(status.lead.text).toBe('Could not check what needs review');
  });

  it('asks for a default currency rather than an empty net worth', () => {
    const book = home({ value: null, unavailable: 'no_reporting_currency' }, { value: null, unavailable: 'no_reporting_currency' });
    expect(briefingStatus(book, null, 'auto', attention([]), 'en').lead.href).toBe('/app/settings/currencies');
  });

  it('pluralizes by the locale rules', async () => {
    const runtime = await import('#lib/paraglide/runtime.js');
    const previous = runtime.getLocale;
    runtime.overwriteGetLocale(() => 'ru');
    try {
      const status = (count: number) => briefingStatus(home(), 'net_worth', 'auto', attention([source('drafts', count)]), 'ru').lead.text;
      expect(status(1)).toBe('На проверку: 1 черновик');
      expect(status(3)).toBe('На проверку: 3 черновика');
      expect(status(5)).toBe('На проверку: 5 черновиков');
    } finally {
      runtime.overwriteGetLocale(previous);
    }
  });
});
