import { afterEach, describe, expect, it, vi } from 'vitest';
import * as runtime from '#lib/paraglide/runtime.js';
import { commodityImbalance, formatEditableAmount, formatLedgerAmount, inflowPositiveAmount, parseDecimalAmount } from './amount.ts';
import { formatQuantity } from './format.ts';
import { parseTradeAmounts } from '#lib/investments/form-amounts.ts';
import { correctionTradeDraft, correctionLotDrafts } from '#lib/investments/trade-economics.ts';
import { parsePriceAmount } from '#lib/pricing/form.ts';
import { parseStatementBalance } from '#lib/reconcile/statement-balance.ts';
import type { InvestmentTradeCorrectionContextResponse } from '#lib/api/investments.ts';
import { exactDecimal } from '#lib/reports/report-csv.ts';

const formattingLocales = ['en', 'es', 'fr', 'nl', 'de', 'ru'] as const;
afterEach(() => vi.restoreAllMocks());

describe('locale-aware editable amounts (#100)', () => {
  it.each(runtime.locales)('uses the active %s locale for entry and exact rehydration', (locale) => {
    vi.spyOn(runtime, 'getLocale').mockReturnValue(locale);
    const decimal = locale === 'en' ? '.' : ',';
    for (const [value, scale] of [['9007199254740993', 2], ['-5', 2], ['0', 2], ['12345678901234567890123456789012345678', 24]] as const) {
      const canonical = formatLedgerAmount(value, scale);
      const editable = formatEditableAmount(value, scale);
      expect(editable).toBe(canonical.replace('.', decimal));
      expect(parseDecimalAmount(editable)).toEqual({ value, scale });
      expect(parseDecimalAmount(formatQuantity(value, scale, locale))).toEqual({ value, scale });
      expect(exactDecimal(value, scale)).toBe(canonical);
    }
    expect(parseDecimalAmount(`-0${decimal}00`)).toEqual({ value: '0', scale: 2 });
    expect(parseDecimalAmount(`1${decimal}123`, { maxScale: 2 })).toBeNull();
    expect(parseDecimalAmount(`0${decimal}${'1'.repeat(25)}`)).toBeNull();
    expect(parseDecimalAmount('9'.repeat(39))).toBeNull();
  });

  it('resolves separators by locale instead of guessing from the last separator', () => {
    expect(parseDecimalAmount('1,234', { locale: 'en' })).toEqual({ value: '1234', scale: 0 });
    expect(parseDecimalAmount('1,234', { locale: 'de' })).toEqual({ value: '1234', scale: 3 });
    expect(parseDecimalAmount('1.234', { locale: 'en' })).toEqual({ value: '1234', scale: 3 });
    expect(parseDecimalAmount('1.234', { locale: 'de' })).toEqual({ value: '1234', scale: 0 });
    expect(parseDecimalAmount('1,50', { locale: 'en' })).toBeNull();
    expect(parseDecimalAmount('1.50', { locale: 'de' })).toBeNull();
  });

  it.each(formattingLocales)('rejects malformed, foreign and mixed grouping in %s', (locale) => {
    const parts = new Intl.NumberFormat(locale).formatToParts(12345.6);
    const group = parts.find(p => p.type === 'group')!.value;
    const decimal = parts.find(p => p.type === 'decimal')!.value;
    for (const input of [`1${group}50`, `12${group}34${group}567`, `1234${group}567`, `1${group}${group}234`, `1${decimal}2${group}345`, `1${decimal}2${decimal}3`, `1${group}234${decimal}`, '+1', '1e3', '−1', '$1', '1\t234', '1\n234']) {
      expect(parseDecimalAmount(input, { locale }), input).toBeNull();
    }
    if (locale !== 'en') expect(parseDecimalAmount('1,234.56', { locale })).toBeNull();
    if (locale !== 'de' && locale !== 'nl' && locale !== 'es') expect(parseDecimalAmount('1.234,56', { locale })).toBeNull();
  });

  it.each(['fr', 'ru'] as const)('accepts keyboard and nonbreaking space grouping in %s', (locale) => {
    for (const space of [' ', '\u00a0', '\u202f']) {
      expect(parseDecimalAmount(`9${space}007${space}199${space}254${space}740${space}993,50`, { locale })).toEqual({ value: '900719925474099350', scale: 2 });
    }
    expect(parseDecimalAmount('1 234\u00a0567,89', { locale })).toBeNull();
  });

  it('uses the active locale throughout investment, price, reconciliation and split helpers', () => {
    vi.spyOn(runtime, 'getLocale').mockReturnValue('nl');
    expect(parseTradeAmounts({ quantityStr: '2,50', cashAmountStr: '9.007.199.254.740,993' })).toEqual({ ok: true, values: { quantity: { value: '250', scale: 2 }, cashAmount: { value: '9007199254740993', scale: 3 } } });
    const source = { quantity_value: '9007199254740993', quantity_scale: 2, net_value: '-12550', net_scale: 2, charges: [], effective_elected_lots: [{ lot_id: 9, quantity_value: '125', quantity_scale: 2 }] } as unknown as InvestmentTradeCorrectionContextResponse;
    expect(correctionTradeDraft(source)).toMatchObject({ quantity: '90071992547409,93', net: '125,50' });
    expect(correctionLotDrafts(source)).toEqual([{ lotID: '9', quantity: '1,25' }]);
    expect(parsePriceAmount('125,50')).toEqual({ value: '12550', scale: 2 });
    expect(parseStatementBalance('-1.234,50')).toEqual({ value: '-123450', scale: 2 });
    expect(inflowPositiveAmount({ quantity_value: '-5', quantity_scale: 2, account_class: 'liability' })).toBe('0,05');
    expect(inflowPositiveAmount({ quantity_value: '0', quantity_scale: 2, account_class: 'liability' })).toBe('0,00');
    expect(commodityImbalance([{ commodityID: '1', amountStr: '1,50' }, { commodityID: '1', amountStr: '-1,5' }])).toBeNull();
    expect(commodityImbalance([{ commodityID: '1', amountStr: '1,50' }])).toEqual({ commodityID: '1', amount: '1,50' });
  });
});
