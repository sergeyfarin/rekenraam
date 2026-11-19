import { describe, expect, it } from 'vitest';
import { parseReconcileRequest, reconcileHref } from './reconcile-link.ts';

describe('reconcileHref', () => {
  it('names the account alone', () => {
    expect(reconcileHref(12)).toBe('/app/ledger/reconcile?account=12');
  });

  it('names the statement commodity too', () => {
    expect(reconcileHref(12, 7)).toBe('/app/ledger/reconcile?account=12&commodity=7');
  });
});

describe('parseReconcileRequest', () => {
  const parse = (query: string) => parseReconcileRequest(new URLSearchParams(query));

  it('round-trips a statement link', () => {
    expect(parse('account=12&commodity=7')).toEqual({ accountID: 12, commodityID: 7 });
  });

  it('reads an account without a commodity', () => {
    expect(parse('account=12')).toEqual({ accountID: 12, commodityID: undefined });
  });

  it('ignores a commodity without an account, and malformed ids', () => {
    expect(parse('commodity=7')).toEqual({});
    expect(parse('account=0')).toEqual({});
    expect(parse('account=12&commodity=-3')).toEqual({ accountID: 12, commodityID: undefined });
    expect(parse('account=1e3&commodity=7')).toEqual({});
  });
});
