import { describe, expect, it } from 'vitest';
import {
  hasPrefill,
  holdingAccountPrefill,
  instrumentPrefill,
  prefilledInstrument,
  prefilledPositionKey,
  restoredPositionKey
} from './investment-prefill';

const instruments = [{ commodity_id: 10 }, { commodity_id: 11 }];
const holding = (id: number, commodity: number, status = 'active', kind = 'security_holding') => ({
  id, account_kind: kind, status, default_commodity_id: commodity
});

describe('holdingAccountPrefill', () => {
  it('pre-selects the holding and its instrument', () => {
    expect(holdingAccountPrefill(holding(1, 10), instruments)).toEqual({ holdingAccountID: 1, commodityID: 10 });
  });

  it('leaves the instrument open when the default commodity is not an instrument', () => {
    expect(holdingAccountPrefill(holding(1, 3), instruments)).toEqual({ holdingAccountID: 1, commodityID: undefined });
  });
});

describe('instrumentPrefill', () => {
  it('pre-selects the only active holding of the instrument', () => {
    const accounts = [holding(1, 10), holding(2, 11), holding(3, 10, 'archived'), holding(4, 10, 'active', 'checking')];
    expect(instrumentPrefill(10, accounts)).toEqual({ commodityID: 10, holdingAccountID: 1 });
  });

  it('leaves the holding open when it is ambiguous', () => {
    expect(instrumentPrefill(10, [holding(1, 10), holding(2, 10, 'active', 'fund_holding')]))
      .toEqual({ commodityID: 10, holdingAccountID: undefined });
    expect(instrumentPrefill(12, [holding(1, 10)])).toEqual({ commodityID: 12, holdingAccountID: undefined });
  });
});

describe('prefilledInstrument', () => {
  it('finds the pre-filled instrument once instruments load', () => {
    expect(prefilledInstrument(undefined, { commodityID: 10 })).toBeUndefined();
    expect(prefilledInstrument(instruments, { commodityID: 11 })).toEqual({ commodity_id: 11 });
    expect(prefilledInstrument(instruments, { holdingAccountID: 1 })).toBeUndefined();
  });
});

describe('prefilledPositionKey', () => {
  const key = (p: { account_id: number; commodity_id: number; cost_commodity_id: number }) =>
    `${p.account_id}:${p.commodity_id}:${p.cost_commodity_id}`;
  const positions = [
    { account_id: 1, commodity_id: 10, cost_commodity_id: 100 },
    { account_id: 2, commodity_id: 10, cost_commodity_id: 100 },
    { account_id: 2, commodity_id: 11, cost_commodity_id: 100 },
    { account_id: 3, commodity_id: 12, cost_commodity_id: 100 },
    { account_id: 3, commodity_id: 12, cost_commodity_id: 101 }
  ];

  it('selects the single position matching every pre-filled field', () => {
    expect(prefilledPositionKey(positions, { holdingAccountID: 1 }, key)).toBe('1:10:100');
    expect(prefilledPositionKey(positions, { holdingAccountID: 2, commodityID: 11 }, key)).toBe('2:11:100');
    expect(prefilledPositionKey(positions, { commodityID: 11 }, key)).toBe('2:11:100');
  });

  it('selects nothing when the match is ambiguous or absent', () => {
    expect(prefilledPositionKey(positions, { commodityID: 10 }, key)).toBe('');
    expect(prefilledPositionKey(positions, { holdingAccountID: 3 }, key)).toBe('');
    expect(prefilledPositionKey(positions, { holdingAccountID: 9 }, key)).toBe('');
    expect(prefilledPositionKey(positions, undefined, key)).toBe('');
    expect(prefilledPositionKey(positions, {}, key)).toBe('');
  });

  it('lets the entry’s shared cost currency pick one of a holding’s currency positions (#508)', () => {
    expect(prefilledPositionKey(positions, { holdingAccountID: 3, currencyID: 101 }, key)).toBe('3:12:101');
    expect(prefilledPositionKey(positions, { holdingAccountID: 3, currencyID: 100 }, key)).toBe('3:12:100');
    // A currency the holding does not carry selects nothing rather than another position.
    expect(prefilledPositionKey(positions, { holdingAccountID: 1, currencyID: 101 }, key)).toBe('');
    // Holdings without a cost currency (corporate actions) ignore it.
    const holdings = [{ account_id: 3, commodity_id: 12 }];
    expect(prefilledPositionKey(holdings, { holdingAccountID: 3, currencyID: 101 }, (h) => `${h.account_id}:${h.commodity_id}`)).toBe('3:12');
  });

  it('keeps an earlier activity’s position identified only when it is restored and fully known (#521 review)', () => {
    const restored = { holdingAccountID: 3, commodityID: 12, currencyID: 101, restoredSelection: true };
    expect(restoredPositionKey(restored, true)).toBe('3:12:101');
    expect(restoredPositionKey(restored, false)).toBe('3:12');
    expect(restoredPositionKey({ ...restored, currencyID: undefined }, true)).toBeUndefined();
    expect(restoredPositionKey({ ...restored, restoredSelection: false }, true)).toBeUndefined();
    expect(restoredPositionKey(undefined, false)).toBeUndefined();
  });

  it('treats duplicate keys as one position', () => {
    const holdings = [{ account_id: 1, commodity_id: 10 }, { account_id: 1, commodity_id: 10 }];
    expect(prefilledPositionKey(holdings, { holdingAccountID: 1 }, (h) => `${h.account_id}:${h.commodity_id}`)).toBe('1:10');
  });

  it('reports whether a prefill carries anything', () => {
    expect(hasPrefill(undefined)).toBe(false);
    expect(hasPrefill({})).toBe(false);
    expect(hasPrefill({ commodityID: 1 })).toBe(true);
  });
});
