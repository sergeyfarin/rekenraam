import { describe, expect, it } from 'vitest';
import type { SpendingReportResponse } from '$lib/api/reports';
import {
  formatShare,
  isSingleCommodity,
  reportCommodityIDs,
  spendingBars,
  spendingDrillDownQuery,
  spendingRows,
  type SpendingRow
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

  it('picks the widest row exactly when coefficients exceed float precision', () => {
    // Two 38-digit coefficients differing only in their last digit: a float
    // compares them as equal and can size both bars against the wrong track.
    const hugeRows = spendingRows(
      report({
        groups: [
          {
            category_id: 1,
            name: 'Smaller',
            category_type: 'expense',
            drill_down: drillDown,
            totals: [
              {
                commodity_id: 1,
                quantity_value: '10000000000000000000000000000000000001',
                quantity_scale: 2
              }
            ]
          },
          {
            category_id: 2,
            name: 'Larger',
            category_type: 'expense',
            drill_down: drillDown,
            totals: [
              {
                commodity_id: 1,
                quantity_value: '10000000000000000000000000000000000002',
                quantity_scale: 2
              }
            ]
          }
        ]
      })
    );

    const bars = spendingBars(hugeRows, (row) => row.name);

    expect(bars[1].ratio).toBe(1);
    expect(bars[0].ratio).toBeLessThan(1);
  });

  it('ranks a scale-24 crypto row against a scale-2 cash row by value', () => {
    // 1.5 units at scale 24 versus 3.00 at scale 2. Aligning to scale 24 makes
    // the crypto coefficient 25 digits wide, so digit count must not decide.
    const cryptoRows = spendingRows(
      report({
        groups: [
          {
            category_id: 1,
            name: 'Crypto',
            category_type: 'expense',
            drill_down: drillDown,
            totals: [
              { commodity_id: 1, quantity_value: '1500000000000000000000000', quantity_scale: 24 }
            ]
          },
          {
            category_id: 2,
            name: 'Cash',
            category_type: 'expense',
            drill_down: drillDown,
            totals: [{ commodity_id: 1, quantity_value: '300', quantity_scale: 2 }]
          }
        ]
      })
    );

    const bars = spendingBars(cryptoRows, (row) => row.name);

    expect(bars[1].ratio).toBe(1);
    expect(bars[0].ratio).toBe(0.5);
  });

  it('sizes no bar for a coefficient the backend could not have produced', () => {
    const malformed = spendingRows(
      report({
        groups: [
          {
            category_id: 1,
            name: 'Malformed',
            category_type: 'expense',
            drill_down: drillDown,
            totals: [{ commodity_id: 1, quantity_value: 'not-a-number', quantity_scale: 2 }]
          },
          {
            category_id: 2,
            name: 'Good',
            category_type: 'expense',
            drill_down: drillDown,
            totals: [{ commodity_id: 1, quantity_value: '10000', quantity_scale: 2 }]
          }
        ]
      })
    );

    const bars = spendingBars(malformed, (row) => row.name);

    expect(bars[0].ratio).toBe(0);
    expect(bars[1].ratio).toBe(1);
  });
});

describe('spendingDrillDownQuery', () => {
  const row = (overrides: Partial<SpendingRow> = {}): SpendingRow => ({
    key: 'k',
    code: '',
    name: 'Groceries',
    commodityID: 1,
    quantityValue: '5000',
    quantityScale: 2,
    unattributed: false,
    drillDown: {
      start_date: '2026-06-01',
      end_date: '2026-06-30',
      category_id: 7,
      account_ids: []
    },
    ...overrides
  });

  it('asks the transactions route the report’s own question', () => {
    const params = spendingDrillDownQuery(row(), 'category', 'spending');
    expect(params?.get('after_date')).toBe('2026-06-01');
    expect(params?.get('before_date')).toBe('2026-06-30');
    expect(params?.get('category_id')).toBe('7');
    // The report sums entry dates and counts posted transactions only.
    expect(params?.get('date_basis')).toBe('entry');
    expect(params?.get('status')).toBe('posted');
  });

  it('carries the direction on a payee grouping, which no category pins', () => {
    const payeeRow = row({ drillDown: { start_date: '2026-06-01', end_date: '2026-06-30', payee_id: 3, account_ids: [] } });
    expect(spendingDrillDownQuery(payeeRow, 'payee', 'spending')?.get('category_type')).toBe('expense');
    expect(spendingDrillDownQuery(payeeRow, 'payee', 'income')?.get('category_type')).toBe('income');
    expect(spendingDrillDownQuery(payeeRow, 'payee', 'spending')?.get('payee_id')).toBe('3');
  });

  it('passes a single resolved account through', () => {
    const scoped = row({ drillDown: { start_date: '2026-06-01', end_date: '2026-06-30', category_id: 7, account_ids: [12] } });
    expect(spendingDrillDownQuery(scoped, 'category', 'spending')?.get('account_id')).toBe('12');
  });

  it('refuses to link what the transactions route cannot express', () => {
    // No "has no category" filter exists.
    expect(spendingDrillDownQuery(row({ unattributed: true }), 'category', 'spending')).toBeUndefined();
    // The route takes one account_id, so several would silently widen the list.
    const manyAccounts = row({ drillDown: { start_date: '2026-06-01', end_date: '2026-06-30', category_id: 7, account_ids: [1, 2] } });
    expect(spendingDrillDownQuery(manyAccounts, 'category', 'spending')).toBeUndefined();
    // A category grouping with no category on the row is not reproducible.
    const noCategory = row({ drillDown: { start_date: '2026-06-01', end_date: '2026-06-30', account_ids: [] } });
    expect(spendingDrillDownQuery(noCategory, 'category', 'spending')).toBeUndefined();
  });
});
