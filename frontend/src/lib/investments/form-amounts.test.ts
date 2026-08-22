import { describe, it, expect } from 'vitest';
import {
  parseDividendAmounts,
  parseMagnitude,
  parseMoneyMagnitude,
  parseTradeAmounts
} from './form-amounts';

/**
 * These tests exist because consolidating the investment forms onto
 * `$lib/money` changes their behaviour rather than preserving it. Each change
 * is pinned by name here so a future edit that reintroduces the old semantics
 * fails loudly instead of quietly.
 */

describe('parseMagnitude', () => {
  it('accepts a plain decimal', () => {
    expect(parseMagnitude('12.50')).toEqual({ ok: true, field: { value: '1250', scale: 2 } });
  });

  it('accepts zero, leaving a zero-amount decision to the endpoint', () => {
    expect(parseMagnitude('0')).toEqual({ ok: true, field: { value: '0', scale: 0 } });
  });

  it('rejects a negative value explicitly rather than as a regex side effect', () => {
    // The old parsers rejected "-5.00" only because they built the coefficient
    // "-500" and tested it with /^\d+$/. parseDecimalAmount handles signs, so
    // without this guard a negative quantity would now be accepted.
    expect(parseMagnitude('-5.00')).toEqual({ ok: false, reason: 'negative' });
  });

  it.each([
    ['a bare minus sign', '-'],
    ['a negative sub-unit value', '-0.05'],
    ['a negative integer', '-1']
  ])('rejects %s', (_name, input) => {
    expect(parseMagnitude(input).ok).toBe(false);
  });

  it('does not treat "-0.00" as negative, since it has no magnitude', () => {
    // parseDecimalAmount canonicalises a zero magnitude to unsigned "0", so
    // this is accepted rather than reported as a negative.
    expect(parseMagnitude('-0.00')).toEqual({ ok: true, field: { value: '0', scale: 2 } });
  });

  it('rejects "1,50" instead of silently reading it as 150', () => {
    // The old parsers stripped every comma, turning a decimal-comma amount
    // into a 100x error — T-45 on the editor side, T-36 on the import side.
    // The user now gets a rejection instead of a hundredfold overstatement.
    expect(parseMagnitude('1,50')).toEqual({ ok: false, reason: 'invalid' });
  });

  it('still accepts correctly grouped thousands', () => {
    expect(parseMagnitude('1,234.56')).toEqual({ ok: true, field: { value: '123456', scale: 2 } });
  });

  it.each([
    ['an empty string', ''],
    ['whitespace only', '   '],
    ['letters', 'abc'],
    ['two decimal points', '1.2.3']
  ])('rejects %s as invalid', (_name, input) => {
    expect(parseMagnitude(input)).toEqual({ ok: false, reason: 'invalid' });
  });

  it('honours a commodity scale ceiling', () => {
    expect(parseMagnitude('1.123', { maxScale: 2 })).toEqual({ ok: false, reason: 'invalid' });
    expect(parseMagnitude('1.12', { maxScale: 2 }).ok).toBe(true);
  });
});

describe('parseMoneyMagnitude', () => {
  it('carries both the exact coefficient and its int64 form', () => {
    expect(parseMoneyMagnitude('12.50')).toEqual({
      ok: true,
      field: { value: '1250', scale: 2, int64: 1250 }
    });
  });

  it('rejects a coefficient a JS number cannot carry losslessly', () => {
    // These API money fields are typed integer/int64, but int64 reaches far
    // past Number.MAX_SAFE_INTEGER. Above the safe range the value must be
    // refused, not silently rounded onto the wire.
    expect(parseMoneyMagnitude('90071992547409.93')).toEqual({ ok: false, reason: 'too_large' });
  });

  it('accepts a value at the top of the safe range', () => {
    // Number.MAX_SAFE_INTEGER is 9007199254740991.
    expect(parseMoneyMagnitude('90071992547409.91')).toEqual({
      ok: true,
      field: { value: '9007199254740991', scale: 2, int64: 9007199254740991 }
    });
  });

  it('reports a negative money amount as negative, not as too large', () => {
    expect(parseMoneyMagnitude('-10.00')).toEqual({ ok: false, reason: 'negative' });
  });
});

describe('parseTradeAmounts (buy and sell)', () => {
  it('returns both magnitudes when the form is valid', () => {
    const result = parseTradeAmounts({ quantityStr: '10', cashAmountStr: '1500.00' });

    expect(result).toEqual({
      ok: true,
      values: {
        quantity: { value: '10', scale: 0 },
        cashAmount: { value: '150000', scale: 2, int64: 150000 }
      }
    });
  });

  it('rejects a negative quantity and names the quantity field', () => {
    // You cannot buy or sell -5 shares. Before this change the rejection was
    // implicit; now it is named, so the form can point at the right input.
    expect(parseTradeAmounts({ quantityStr: '-5', cashAmountStr: '100.00' })).toEqual({
      ok: false,
      field: 'quantity',
      reason: 'negative'
    });
  });

  it('rejects a negative cash amount and names the cash field', () => {
    expect(parseTradeAmounts({ quantityStr: '5', cashAmountStr: '-100.00' })).toEqual({
      ok: false,
      field: 'cash_amount',
      reason: 'negative'
    });
  });

  it('rejects a decimal-comma quantity rather than inflating it 100x', () => {
    expect(parseTradeAmounts({ quantityStr: '1,50', cashAmountStr: '100.00' })).toEqual({
      ok: false,
      field: 'quantity',
      reason: 'invalid'
    });
  });

  it('reports the quantity first when both fields are bad', () => {
    // Deterministic ordering keeps the surfaced error stable rather than
    // depending on evaluation order.
    expect(parseTradeAmounts({ quantityStr: 'abc', cashAmountStr: 'abc' })).toEqual({
      ok: false,
      field: 'quantity',
      reason: 'invalid'
    });
  });

  it('keeps a fractional share quantity exact', () => {
    const result = parseTradeAmounts({ quantityStr: '0.00000001', cashAmountStr: '1.00' });

    expect(result).toMatchObject({ ok: true, values: { quantity: { value: '1', scale: 8 } } });
  });

  it('lets a quantity exceed the int64 money cap, since it travels as a string', () => {
    // quantity_value is a lossless coefficient string on the wire, so it must
    // not inherit the money fields' safe-integer limit.
    const result = parseTradeAmounts({ quantityStr: '90071992547409.93', cashAmountStr: '1.00' });

    expect(result.ok).toBe(true);
  });
});

describe('parseDividendAmounts', () => {
  const base = {
    amountStr: '100.00',
    withholdingStr: '',
    includeWithholding: false,
    quantityStr: '',
    includeQuantity: false
  };

  it('validates a cash dividend with only an amount', () => {
    expect(parseDividendAmounts(base)).toEqual({
      ok: true,
      values: {
        amount: { value: '10000', scale: 2, int64: 10000 },
        withholding: null,
        quantity: null
      }
    });
  });

  it('treats a blank withholding field as absent, not invalid', () => {
    const result = parseDividendAmounts({ ...base, includeWithholding: true, withholdingStr: '   ' });

    expect(result).toMatchObject({ ok: true, values: { withholding: null } });
  });

  it('validates withholding when it is filled in', () => {
    const result = parseDividendAmounts({ ...base, includeWithholding: true, withholdingStr: '15.00' });

    expect(result).toMatchObject({ ok: true, values: { withholding: { value: '1500', scale: 2, int64: 1500 } } });
  });

  it('ignores a withholding value when the section is closed', () => {
    // The old form only parsed withholding when the section was open; a stale
    // value left behind by closing it must not reach the request.
    const result = parseDividendAmounts({ ...base, includeWithholding: false, withholdingStr: '-99' });

    expect(result).toMatchObject({ ok: true, values: { withholding: null } });
  });

  it('rejects a negative withholding and names the withholding field', () => {
    expect(parseDividendAmounts({ ...base, includeWithholding: true, withholdingStr: '-15.00' })).toEqual({
      ok: false,
      field: 'withholding',
      reason: 'negative'
    });
  });

  it('requires a quantity in reinvested mode', () => {
    expect(parseDividendAmounts({ ...base, includeQuantity: true, quantityStr: '' })).toEqual({
      ok: false,
      field: 'quantity',
      reason: 'invalid'
    });
  });

  it('validates a reinvested dividend end to end', () => {
    const result = parseDividendAmounts({
      ...base,
      includeQuantity: true,
      quantityStr: '2.5'
    });

    expect(result).toMatchObject({
      ok: true,
      values: { amount: { int64: 10000 }, quantity: { value: '25', scale: 1 } }
    });
  });

  it('rejects a negative reinvested quantity and names the quantity field', () => {
    expect(parseDividendAmounts({ ...base, includeQuantity: true, quantityStr: '-2.5' })).toEqual({
      ok: false,
      field: 'quantity',
      reason: 'negative'
    });
  });

  it('rejects a decimal-comma dividend amount rather than inflating it 100x', () => {
    expect(parseDividendAmounts({ ...base, amountStr: '1,50' })).toEqual({
      ok: false,
      field: 'amount',
      reason: 'invalid'
    });
  });

  it('reports the amount before the withholding when both are bad', () => {
    expect(
      parseDividendAmounts({ ...base, amountStr: 'abc', includeWithholding: true, withholdingStr: 'abc' })
    ).toEqual({ ok: false, field: 'amount', reason: 'invalid' });
  });
});
