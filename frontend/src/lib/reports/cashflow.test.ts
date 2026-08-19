import { describe, expect, it } from 'vitest';
import type { CashflowReportResponse } from '$lib/api/reports';
import {
  cashflowDrillDownHref,
  cashflowRows,
  hasMultipleCommodities,
  isEmptyCashflow,
  transferDetail
} from './cashflow';

function quantity(commodityID: number, value: string, scale = 2) {
  return {
    commodity_id: commodityID,
    quantity_value: value,
    quantity_scale: scale,
    normal_quantity_value: value
  };
}

function report(buckets: CashflowReportResponse['buckets']): CashflowReportResponse {
  return {
    query: {
      start_date: '2026-06-01',
      end_date: '2026-06-30',
      bucket: 'month',
      cash_scope: 'default_liquid_cash',
      default_cash_account_kinds: ['brokerage_cash', 'cash', 'checking', 'savings'],
      filters: {
        account_ids: [],
        include_descendants: false,
        commodity_ids: [],
        resolved_account_ids: [4, 5]
      }
    },
    buckets,
    excluded_system_roles: ['all']
  };
}

const emptyBucket = {
  start_date: '2026-06-01',
  end_date: '2026-06-30',
  inflow: [],
  outflow: [],
  transfer_in: [],
  transfer_out: [],
  operating_net: [],
  transfer_net: [],
  net_movement: []
};

describe('cashflowRows', () => {
  it('builds one row per bucket per commodity', () => {
    const rows = cashflowRows(
      report([
        {
          ...emptyBucket,
          inflow: [quantity(1, '200000')],
          outflow: [quantity(1, '12000')],
          operating_net: [quantity(1, '188000')],
          net_movement: [quantity(1, '188000')]
        }
      ])
    );

    expect(rows).toHaveLength(1);
    expect(rows[0].inflow).toBe('200000');
    expect(rows[0].outflow).toBe('12000');
    expect(rows[0].operatingNet).toBe('188000');
    expect(rows[0].netMovement).toBe('188000');
    expect(rows[0].quantityScale).toBe(2);
  });

  it('reads a measure absent for a commodity as zero, not as a missing row', () => {
    // A month with only a card payment: financing movement and a net movement,
    // but no income or expense counterpart at all.
    const rows = cashflowRows(
      report([
        {
          ...emptyBucket,
          transfer_out: [quantity(1, '5000')],
          transfer_net: [quantity(1, '-5000')],
          net_movement: [quantity(1, '-5000')]
        }
      ])
    );

    expect(rows).toHaveLength(1);
    expect(rows[0].inflow).toBe('0');
    expect(rows[0].outflow).toBe('0');
    expect(rows[0].transferOut).toBe('5000');
    expect(rows[0].netMovement).toBe('-5000');
  });

  it('keeps unlike commodities as separate rows', () => {
    const rows = cashflowRows(
      report([
        {
          ...emptyBucket,
          inflow: [quantity(2, '9000'), quantity(1, '200000')],
          net_movement: [quantity(1, '200000'), quantity(2, '9000')]
        }
      ])
    );

    expect(rows.map((row) => row.commodityID)).toEqual([1, 2]);
    expect(hasMultipleCommodities(rows)).toBe(true);
  });

  it('takes the scale from whichever measure is present', () => {
    const rows = cashflowRows(
      report([{ ...emptyBucket, net_movement: [quantity(1, '1234567', 4)] }])
    );
    expect(rows[0].quantityScale).toBe(4);
  });

  it('emits a row for a bucket whose movements cancelled to zero', () => {
    // An internal transfer nets to an explicit zero. That is activity, and
    // hiding the row would claim the month was quiet.
    const rows = cashflowRows(report([{ ...emptyBucket, net_movement: [quantity(1, '0')] }]));
    expect(rows).toHaveLength(1);
    expect(rows[0].netMovement).toBe('0');
    expect(isEmptyCashflow(rows)).toBe(false);
  });

  it('reports an empty range as empty', () => {
    expect(isEmptyCashflow(cashflowRows(report([emptyBucket])))).toBe(true);
  });
});

describe('transferDetail', () => {
  const rowWith = (fields: Record<string, string>) =>
    cashflowRows(
      report([
        {
          ...emptyBucket,
          ...Object.fromEntries(
            Object.entries(fields).map(([field, value]) => [field, [quantity(1, value)]])
          )
        }
      ])
    )[0];

  it('names only the side that moved', () => {
    expect(transferDetail(rowWith({ transfer_in: '100000', net_movement: '100000' }))).toBe('in');
    expect(transferDetail(rowWith({ transfer_out: '75000', net_movement: '-75000' }))).toBe('out');
    expect(transferDetail(rowWith({ transfer_in: '120000', transfer_out: '75000', net_movement: '45000' }))).toBe('both');
  });

  it('is none when only income and expense moved', () => {
    expect(transferDetail(rowWith({ inflow: '100000', net_movement: '100000' }))).toBe('none');
  });
});

describe('cashflowDrillDownHref', () => {
  const [row] = cashflowRows(
    report([
      {
        ...emptyBucket,
        inflow: [quantity(1, '320000')],
        outflow: [quantity(1, '41250')],
        operating_net: [quantity(1, '278750')],
        net_movement: [quantity(1, '278750')]
      }
    ])
  );

  it('sends the outflow to spending over the same range and accounts', () => {
    const href = cashflowDrillDownHref(row, 'out', [4, 5], 'week');
    const params = new URLSearchParams(href?.split('?')[1]);
    expect(params.get('view')).toBe('spending');
    expect(params.get('mode')).toBe('spending');
    expect(params.get('start_date')).toBe('2026-06-01');
    expect(params.get('end_date')).toBe('2026-06-30');
    // The resolved scope the cashflow report actually measured, not a default
    // re-derived later.
    expect(params.getAll('account_id')).toEqual(['4', '5']);
    // The row's own commodity only.
    expect(params.getAll('commodity_id')).toEqual(['1']);
    expect(params.get('bucket')).toBe('week');
  });

  it('sends the inflow to the income direction', () => {
    const href = cashflowDrillDownHref(row, 'in', [4], 'month');
    expect(new URLSearchParams(href?.split('?')[1]).get('mode')).toBe('income');
  });

  it('does not link a direction where nothing moved', () => {
    const [quiet] = cashflowRows(
      report([{ ...emptyBucket, transfer_out: [quantity(1, '5000')], net_movement: [quantity(1, '-5000')] }])
    );
    expect(cashflowDrillDownHref(quiet, 'in', [4], 'month')).toBeUndefined();
    expect(cashflowDrillDownHref(quiet, 'out', [4], 'month')).toBeUndefined();
  });
});
