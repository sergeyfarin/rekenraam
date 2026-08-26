import { describe, expect, it } from 'vitest';
import type { CashflowReportResponse } from '$lib/api/reports';
import {
  cashflowDrillDownHref,
  cashflowRows,
  convertedSeries,
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
    expect(rows[0].scales.netMovement).toBe(4);
  });

  it('keeps each measure with the scale it was produced at', () => {
    // The server writes one scale per commodity, but a client that pairs one
    // measure's coefficient with another's scale renders 100 as 1.00. Reading
    // each measure's own scale makes that impossible rather than unlikely.
    const rows = cashflowRows(
      report([
        {
          ...emptyBucket,
          inflow: [quantity(1, '5000', 2)],
          outflow: [quantity(1, '100', 0)],
          net_movement: [quantity(1, '-5000', 2)]
        }
      ])
    );

    expect(rows[0].scales.inflow).toBe(2);
    expect(rows[0].scales.outflow).toBe(0);
    expect(rows[0].scales.netMovement).toBe(2);
    // The row-level scale is the deepest present, never a sibling's.
    expect(rows[0].quantityScale).toBe(2);
  });

  it('gives an absent measure the row scale, since zero is zero at any scale', () => {
    const rows = cashflowRows(
      report([{ ...emptyBucket, net_movement: [quantity(1, '-5000', 2)] }])
    );
    expect(rows[0].inflow).toBe('0');
    expect(rows[0].scales.inflow).toBe(2);
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

describe('the reporting-currency restatement', () => {
  const convertedBucket = (overrides: Record<string, unknown> = {}) => ({
    ...emptyBucket,
    inflow: [quantity(1, '200000')],
    outflow: [quantity(1, '12000')],
    operating_net: [quantity(1, '188000')],
    net_movement: [quantity(1, '188000')],
    converted_inflow: quantity(9, '180000'),
    converted_outflow: quantity(9, '10800'),
    converted_transfer_in: quantity(9, '0'),
    converted_transfer_out: quantity(9, '0'),
    converted_operating_net: quantity(9, '169200'),
    converted_transfer_net: quantity(9, '0'),
    converted_net_movement: quantity(9, '169200'),
    ...overrides
  });

  it('follows the commodity rows with a full seven-measure row', () => {
    const rows = cashflowRows(report([convertedBucket()]));
    expect(rows).toHaveLength(2);
    expect(rows[0].converted).toBe(false);
    const restatement = rows[1];
    expect(restatement.converted).toBe(true);
    expect(restatement.commodityID).toBe(9);
    expect(restatement.inflow).toBe('180000');
    expect(restatement.netMovement).toBe('169200');
    // Every measure is rounded once into the reporting currency's own scale, so
    // no measure may inherit a scale from a commodity row.
    expect(new Set(Object.values(restatement.scales))).toEqual(new Set([2]));
  });

  it('holds its own identities exactly', () => {
    const restatement = cashflowRows(report([convertedBucket()])).at(-1)!;
    expect(BigInt(restatement.operatingNet)).toBe(
      BigInt(restatement.inflow) - BigInt(restatement.outflow)
    );
    expect(BigInt(restatement.netMovement)).toBe(
      BigInt(restatement.operatingNet) + BigInt(restatement.transferNet)
    );
  });

  it('is omitted whole when any one measure did not convert', () => {
    // A real inflow beside a zero outflow that only means "unknown" would make
    // the net movement between them neither figure.
    const rows = cashflowRows(report([convertedBucket({ converted_outflow: undefined })]));
    expect(rows.every((row) => !row.converted)).toBe(true);
  });

  it('is not a commodity anyone holds', () => {
    const rows = cashflowRows(report([convertedBucket()]));
    expect(hasMultipleCommodities(rows)).toBe(false);
  });

  it('charts nothing when a bucket did not convert', () => {
    const rows = cashflowRows(
      report([
        convertedBucket(),
        { ...convertedBucket({ converted_net_movement: undefined }), start_date: '2026-07-01', end_date: '2026-07-31' }
      ])
    );
    expect(convertedSeries(rows)).toEqual([]);
  });
});

describe('a restatement is not a set of postings', () => {
  const bucket = {
    ...emptyBucket,
    inflow: [quantity(1, '200000')],
    outflow: [quantity(1, '12000')],
    operating_net: [quantity(1, '188000')],
    net_movement: [quantity(1, '188000')],
    converted_inflow: quantity(9, '180000'),
    converted_outflow: quantity(9, '10800'),
    converted_transfer_in: quantity(9, '0'),
    converted_transfer_out: quantity(9, '0'),
    converted_operating_net: quantity(9, '169200'),
    converted_transfer_net: quantity(9, '0'),
    converted_net_movement: quantity(9, '169200')
  };

  it('offers no drill-down, which would narrow to the reporting currency', () => {
    const rows = cashflowRows(report([bucket]));
    expect(cashflowDrillDownHref(rows[0], 'in', [4], 'month')).toBeDefined();
    expect(cashflowDrillDownHref(rows[1], 'in', [4], 'month')).toBeUndefined();
  });

  it('does not turn a range with no activity into a table of zeros', () => {
    // The backend converts an empty bucket to an exact zero, which is true but
    // is not activity.
    const rows = cashflowRows(
      report([
        {
          ...emptyBucket,
          converted_inflow: quantity(9, '0'),
          converted_outflow: quantity(9, '0'),
          converted_transfer_in: quantity(9, '0'),
          converted_transfer_out: quantity(9, '0'),
          converted_operating_net: quantity(9, '0'),
          converted_transfer_net: quantity(9, '0'),
          converted_net_movement: quantity(9, '0')
        }
      ])
    );
    expect(isEmptyCashflow(rows)).toBe(true);
  });
});
