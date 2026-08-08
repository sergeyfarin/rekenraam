import { describe, it, expect } from 'vitest';
import type { CashflowResponse } from '$lib/api/ledger';
import { cashflowIsEmpty, cashflowIsMultiCommodity, cashflowRows, isNegativeAmount } from './cashflow';

function total(commodityId: number, netMovement: string) {
  return {
    commodity_id: commodityId,
    quantity_scale: 2,
    inflow: '200000',
    outflow: '90000',
    operating_net: '110000',
    transfer_in: '0',
    transfer_out: '40000',
    net_movement: netMovement
  };
}

function report(buckets: CashflowResponse['buckets']): CashflowResponse {
  return {
    start_date: '2026-06-01',
    end_date: '2026-08-31',
    bucket: 'month',
    query: { start_date: '2026-06-01', end_date: '2026-08-31', bucket: 'month' },
    scope_kinds: ['cash', 'checking', 'savings', 'brokerage_cash'],
    scope_accounts: [{ account_id: 1, name: 'Checking', account_kind: 'checking' }],
    buckets,
    excluded_system_roles: ['commodity_trading']
  } as CashflowResponse;
}

describe('cashflowRows', () => {
  it('emits one row per commodity in a bucket', () => {
    const rows = cashflowRows(
      report([{ start_date: '2026-06-01', end_date: '2026-06-30', totals: [total(1, '70000'), total(2, '500')] }])
    );

    expect(rows.map((row) => row.key)).toEqual(['2026-06-01:2026-06-30:1', '2026-06-01:2026-06-30:2']);
  });

  it('keeps a bucket with no movement as a row rather than dropping it', () => {
    // A gap in a cashflow table reads as missing data, not as a quiet month.
    const rows = cashflowRows(
      report([
        { start_date: '2026-06-01', end_date: '2026-06-30', totals: [total(1, '70000')] },
        { start_date: '2026-07-01', end_date: '2026-07-31', totals: [] }
      ])
    );

    expect(rows).toHaveLength(2);
    expect(rows[1].commodityId).toBeNull();
    expect(rows[1].startDate).toBe('2026-07-01');
  });

  it('carries every classified figure through unchanged', () => {
    // The frontend does no arithmetic here; the identity already holds.
    const rows = cashflowRows(
      report([{ start_date: '2026-06-01', end_date: '2026-06-30', totals: [total(1, '70000')] }])
    );

    expect(rows[0]).toMatchObject({
      inflow: '200000',
      outflow: '90000',
      operatingNet: '110000',
      transferIn: '0',
      transferOut: '40000',
      netMovement: '70000',
      quantityScale: 2
    });
  });
});

describe('cashflowIsEmpty', () => {
  it('is true when no bucket has movement', () => {
    expect(
      cashflowIsEmpty(
        report([
          { start_date: '2026-06-01', end_date: '2026-06-30', totals: [] },
          { start_date: '2026-07-01', end_date: '2026-07-31', totals: [] }
        ])
      )
    ).toBe(true);
  });

  it('is false as soon as one bucket has movement', () => {
    expect(
      cashflowIsEmpty(
        report([
          { start_date: '2026-06-01', end_date: '2026-06-30', totals: [] },
          { start_date: '2026-07-01', end_date: '2026-07-31', totals: [total(1, '70000')] }
        ])
      )
    ).toBe(false);
  });
});

describe('cashflowIsMultiCommodity', () => {
  it('counts commodities across all buckets, not within one', () => {
    // Two single-commodity buckets in different currencies still make the
    // report multi-commodity.
    const multi = report([
      { start_date: '2026-06-01', end_date: '2026-06-30', totals: [total(1, '70000')] },
      { start_date: '2026-07-01', end_date: '2026-07-31', totals: [total(2, '500')] }
    ]);

    expect(cashflowIsMultiCommodity(multi)).toBe(true);
  });

  it('is false when every bucket uses the same commodity', () => {
    const single = report([
      { start_date: '2026-06-01', end_date: '2026-06-30', totals: [total(1, '70000')] },
      { start_date: '2026-07-01', end_date: '2026-07-31', totals: [total(1, '500')] }
    ]);

    expect(cashflowIsMultiCommodity(single)).toBe(false);
  });
});

describe('isNegativeAmount', () => {
  it.each([
    ['a negative coefficient', '-70000', true],
    ['a positive coefficient', '70000', false],
    ['zero', '0', false],
    // Reading the sign from the string keeps values past
    // Number.MAX_SAFE_INTEGER correct.
    ['a coefficient beyond Number.MAX_SAFE_INTEGER', '-9007199254740993000', true]
  ])('detects %s', (_name, value, expected) => {
    expect(isNegativeAmount(value)).toBe(expected);
  });
});
