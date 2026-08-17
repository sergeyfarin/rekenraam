import { describe, expect, it } from 'vitest';
import type { SpendingReportResponse } from '$lib/api/reports';
import {
  formatShare,
  isSingleCommodity,
  reportCommodityIDs,
  spendingBars,
  spendingRows
} from './spending';

function report(overrides: Partial<SpendingReportResponse> = {}): SpendingReportResponse {
  return {
    query: {
      start_date: '2026-06-01',
      end_date: '2026-06-30',
      group_by: 'category',
      mode: 'spending',
      category_ids: [],
      payee_ids: [],
      filters: {
        account_ids: [],
        include_descendants: false,
        commodity_ids: [],
        resolved_account_ids: []
      }
    },
    groups: [],
    commodity_totals: [],
    excluded_system_roles: [],
    grouping_policy: 'direct_postings',
    ...overrides
  };
}

const drillDown = { start_date: '2026-06-01', end_date: '2026-06-30', account_ids: [] };

describe('spendingRows', () => {
  it('emits one row per group and commodity without combining unlike commodities', () => {
    const rows = spendingRows(
      report({
        groups: [
          {
            category_id: 7,
            code: 'expense_travel',
            name: 'Travel',
            category_type: 'expense',
            drill_down: drillDown,
            totals: [
              { commodity_id: 1, quantity_value: '30000', quantity_scale: 2, share_basis_points: 7500 },
              { commodity_id: 2, quantity_value: '5000', quantity_scale: 2, share_basis_points: 10000 }
            ]
          }
        ]
      })
    );

    expect(rows).toHaveLength(2);
    expect(rows.map((row) => row.commodityID)).toEqual([1, 2]);
    expect(new Set(rows.map((row) => row.key)).size).toBe(2);
    expect(rows[0].name).toBe('Travel');
    expect(rows[0].unattributed).toBe(false);
  });

  it('marks the group holding postings with no payee record', () => {
    const rows = spendingRows(
      report({
        query: { ...report().query, group_by: 'payee' },
        groups: [
          {
            category_type: 'expense',
            drill_down: drillDown,
            totals: [{ commodity_id: 1, quantity_value: '30000', quantity_scale: 2 }]
          }
        ]
      })
    );

    expect(rows).toHaveLength(1);
    expect(rows[0].unattributed).toBe(true);
    expect(rows[0].payeeID).toBeUndefined();
  });

  it('keeps keys unique when several groups are unattributed', () => {
    const rows = spendingRows(
      report({
        groups: [
          {
            category_type: 'expense',
            drill_down: drillDown,
            totals: [{ commodity_id: 1, quantity_value: '100', quantity_scale: 2 }]
          },
          {
            category_type: 'expense',
            drill_down: drillDown,
            totals: [{ commodity_id: 1, quantity_value: '200', quantity_scale: 2 }]
          }
        ]
      })
    );

    expect(new Set(rows.map((row) => row.key)).size).toBe(2);
  });
});

describe('reportCommodityIDs', () => {
  it('detects a single-commodity report so a chart summary stays honest', () => {
    const single = report({
      commodity_totals: [
        { commodity_id: 1, quantity_value: '40000', quantity_scale: 2, normal_quantity_value: '40000' }
      ]
    });
    const multi = report({
      commodity_totals: [
        { commodity_id: 1, quantity_value: '40000', quantity_scale: 2, normal_quantity_value: '40000' },
        { commodity_id: 2, quantity_value: '5000', quantity_scale: 2, normal_quantity_value: '5000' }
      ]
    });

    expect(reportCommodityIDs(single)).toEqual([1]);
    expect(isSingleCommodity(single)).toBe(true);
    expect(isSingleCommodity(multi)).toBe(false);
    expect(isSingleCommodity(report())).toBe(false);
  });
});

describe('formatShare', () => {
  it('presents server-computed basis points as a percentage', () => {
    expect(formatShare(7500, 'en-US')).toBe('75.0%');
    expect(formatShare(3333, 'en-US')).toBe('33.3%');
    expect(formatShare(10000, 'en-US')).toBe('100.0%');
  });

  it('renders nothing when the server omitted the share', () => {
    expect(formatShare(undefined, 'en-US')).toBe('');
  });

  it('keeps a negative share signed rather than dropping the sign', () => {
    expect(formatShare(-2500, 'en-US')).toBe('-25.0%');
  });
});

describe('spendingBars', () => {
  const rows = spendingRows(
    report({
      groups: [
        {
          category_id: 7,
          name: 'Travel',
          category_type: 'expense',
          drill_down: drillDown,
          totals: [{ commodity_id: 1, quantity_value: '30000', quantity_scale: 2, share_basis_points: 7500 }]
        },
        {
          category_id: 8,
          name: 'Groceries',
          category_type: 'expense',
          drill_down: drillDown,
          totals: [{ commodity_id: 1, quantity_value: '10000', quantity_scale: 2, share_basis_points: 2500 }]
        }
      ]
    })
  );

  it('scales bars against the widest magnitude', () => {
    const bars = spendingBars(rows, (row) => row.name);

    expect(bars.map((bar) => bar.label)).toEqual(['Travel', 'Groceries']);
    expect(bars[0].ratio).toBe(1);
    expect(bars[1].ratio).toBeCloseTo(1 / 3, 10);
    expect(bars.every((bar) => bar.negative === false)).toBe(true);
  });

  it('scales by magnitude and flags negative rows for a non-colour cue', () => {
    const withRefund = spendingRows(
      report({
        groups: [
          {
            category_id: 9,
            name: 'Refunded',
            category_type: 'expense',
            drill_down: drillDown,
            totals: [{ commodity_id: 1, quantity_value: '-20000', quantity_scale: 2 }]
          },
          {
            category_id: 8,
            name: 'Groceries',
            category_type: 'expense',
            drill_down: drillDown,
            totals: [{ commodity_id: 1, quantity_value: '10000', quantity_scale: 2 }]
          }
        ]
      })
    );

    const bars = spendingBars(withRefund, (row) => row.name);

    expect(bars[0].negative).toBe(true);
    expect(bars[0].ratio).toBe(1);
    expect(bars[1].ratio).toBe(0.5);
  });

  it('keeps zero rows visible instead of disagreeing with the table', () => {
    const zeroRows = spendingRows(
      report({
        groups: [
          {
            category_id: 8,
            name: 'Net zero',
            category_type: 'expense',
            drill_down: drillDown,
            totals: [{ commodity_id: 1, quantity_value: '0', quantity_scale: 2 }]
          }
        ]
      })
    );

    const bars = spendingBars(zeroRows, (row) => row.name);

    expect(bars).toHaveLength(1);
    expect(bars[0].ratio).toBe(0);
  });

  it('compares magnitudes across differing scales', () => {
    const mixedScales = spendingRows(
      report({
        groups: [
          {
            category_id: 1,
            name: 'Scale three',
            category_type: 'expense',
            drill_down: drillDown,
            totals: [{ commodity_id: 1, quantity_value: '100000', quantity_scale: 3 }]
          },
          {
            category_id: 2,
            name: 'Scale two',
            category_type: 'expense',
            drill_down: drillDown,
            totals: [{ commodity_id: 1, quantity_value: '5000', quantity_scale: 2 }]
          }
        ]
      })
    );

    const bars = spendingBars(mixedScales, (row) => row.name);

    // 100.000 versus 50.00 — the higher scale must not win on digit count.
    expect(bars[0].ratio).toBe(1);
    expect(bars[1].ratio).toBe(0.5);
  });
});
