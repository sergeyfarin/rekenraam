import { describe, expect, it } from 'vitest';
import { chooseStatementCommodity, statementCommodityChoice, statementCommodityOptions } from './statement-commodity.ts';

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
