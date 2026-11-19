import { describe, expect, it } from 'vitest';
import {
  chooseStatementCommodity,
  defaultStatementCommodity,
  lookupStatementCommodities,
  needsCommodityLookup,
  statementCommodityChoice,
  statementCommodityOptions
} from './statement-commodity.ts';

const usd = { id: 1, code: 'USD' };
const judged = [{ commodity_id: 2, commodity_code: 'EUR' }, { commodity_id: 1, commodity_code: 'USD' }];

describe('statementCommodityOptions', () => {
  it('offers the default currency and every judged commodity once', () => {
    expect(statementCommodityOptions(usd, judged)).toEqual([usd, { id: 2, code: 'EUR' }]);
  });

  it('offers the posted commodities of an account without a default currency', () => {
    expect(statementCommodityOptions(undefined, judged)).toEqual([{ id: 2, code: 'EUR' }, usd]);
  });
});

describe('chooseStatementCommodity', () => {
  const options = statementCommodityOptions(usd, judged);
  it('keeps the owner choice, then the request, then the default', () => {
    expect(chooseStatementCommodity(options, 2, 1, 1)).toBe(2);
    expect(chooseStatementCommodity(options, undefined, 2, 1)).toBe(2);
    expect(chooseStatementCommodity(options, undefined, undefined, 1)).toBe(1);
  });

  it('ignores a choice or request the account does not hold', () => {
    expect(chooseStatementCommodity(options, 9, 8, 1)).toBe(1);
  });

  it('falls back to the weakest judged commodity without a default', () => {
    expect(chooseStatementCommodity(statementCommodityOptions(undefined, judged), undefined, undefined, null)).toBe(2);
    expect(chooseStatementCommodity([], undefined, 3, null)).toBeUndefined();
  });
});

describe('statementCommodityChoice', () => {
  const both = statementCommodityOptions(undefined, judged);
  it('keeps a locked commodity although a later lookup would choose another', () => {
    expect(statementCommodityChoice(both, 1, 2, null)).toEqual({ commodityID: 1, lost: false });
  });

  it('asks for a new choice when the locked commodity is no longer offered, never switching on its own', () => {
    expect(statementCommodityChoice([{ id: 2, code: 'EUR' }], 1, 2, null)).toEqual({ commodityID: undefined, lost: true });
  });

  it('settles the first choice from the request or default before anything is locked', () => {
    expect(statementCommodityChoice(both, undefined, 2, null)).toEqual({ commodityID: 2, lost: false });
  });
});

describe('lookupStatementCommodities', () => {
  it('offers a future statement\'s currency the judgement leaves out, so a register link opens in it (#594 review)', () => {
    const lookup = {
      reconciliation: { commodities: [{ commodity_id: 1, commodity_code: 'USD' }] },
      statement_commodities: [{ commodity_id: 3, commodity_code: 'CHF' }]
    };
    const options = statementCommodityOptions(usd, lookupStatementCommodities(lookup));
    expect(options).toEqual([usd, { id: 3, code: 'CHF' }]);
    expect(statementCommodityChoice(options, undefined, 3, 1)).toEqual({ commodityID: 3, lost: false });
  });

  it('offers statement currencies of an account that cannot be judged', () => {
    expect(lookupStatementCommodities({ reconciliation: null, statement_commodities: [{ commodity_id: 3, commodity_code: 'CHF' }] }))
      .toEqual([{ commodity_id: 3, commodity_code: 'CHF' }]);
  });
});

describe('a linked holding statement in another commodity (#594 review)', () => {
  const instrument = 40;
  const lookup = { reconciliation: null, statement_commodities: [{ commodity_id: 1, commodity_code: 'USD' }], default_commodity: { commodity_id: instrument, commodity_code: 'ACME' } };

  it('reads the lookup only when the link names a commodity other than the instrument', () => {
    expect(needsCommodityLookup({ subledgerManaged: true, requestedCommodityID: 1, defaultID: instrument })).toBe(true);
    expect(needsCommodityLookup({ subledgerManaged: true, requestedCommodityID: instrument, defaultID: instrument })).toBe(false);
    expect(needsCommodityLookup({ subledgerManaged: true, requestedCommodityID: undefined, defaultID: instrument })).toBe(false);
    expect(needsCommodityLookup({ subledgerManaged: false, requestedCommodityID: undefined, defaultID: 1 })).toBe(true);
  });

  it('opens on the linked cash statement and names the instrument by its code', () => {
    const fallback = defaultStatementCommodity(instrument, undefined, lookup.default_commodity);
    expect(fallback).toEqual({ id: instrument, code: 'ACME' });
    const options = statementCommodityOptions(fallback, lookupStatementCommodities(lookup));
    expect(options).toEqual([{ id: instrument, code: 'ACME' }, { id: 1, code: 'USD' }]);
    expect(statementCommodityChoice(options, undefined, 1, instrument)).toEqual({ commodityID: 1, lost: false });
  });

  it('prefers the currency list\'s code and ignores a lookup default for another commodity', () => {
    expect(defaultStatementCommodity(1, 'USD', { commodity_id: 1, commodity_code: 'X' })).toEqual({ id: 1, code: 'USD' });
    expect(defaultStatementCommodity(1, undefined, { commodity_id: 2, commodity_code: 'EUR' })).toEqual({ id: 1, code: '' });
    expect(defaultStatementCommodity(null, 'USD', null)).toBeUndefined();
  });
});
