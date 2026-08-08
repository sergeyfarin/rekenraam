import { describe, it, expect } from 'vitest';
import type { SpendingGroup, SpendingResponse } from '$lib/api/ledger';
import { formatShare, spendingIsMultiCommodity, spendingRows } from './spending';

function total(commodityId: number, value: string, shareBasisPoints: number) {
  return {
    commodity_id: commodityId,
    quantity_value: value,
    quantity_scale: 2,
    normal_quantity_value: value,
    share_basis_points: shareBasisPoints
  };
}

function report(groups: SpendingGroup[], overrides: Partial<SpendingResponse> = {}): SpendingResponse {
  return {
    start_date: '2026-06-01',
    end_date: '2026-06-30',
    group_by: 'category',
    direction: 'expense',
    rank_commodity_id: 1,
    query: {
      start_date: '2026-06-01',
      end_date: '2026-06-30',
      group_by: 'category',
      direction: 'expense'
    },
    commodity_totals: [
      { commodity_id: 1, quantity_value: '120000', quantity_scale: 2, normal_quantity_value: '120000' }
    ],
    groups,
    excluded_system_roles: ['commodity_trading'],
    ...overrides
  } as SpendingResponse;
}

describe('spendingRows', () => {
  it('preserves the backend ranking order', () => {
    const rows = spendingRows(
      report([
        { category_account_id: 7, unassigned: false, totals: [total(1, '90000', 7500)] },
        { category_account_id: 3, unassigned: false, totals: [total(1, '30000', 2500)] }
      ])
    );

    expect(rows.map((row) => row.normalQuantityValue)).toEqual(['90000', '30000']);
  });

  it('emits one row per commodity within a group and keeps them adjacent', () => {
    const rows = spendingRows(
      report([
        {
          category_account_id: 7,
          unassigned: false,
          totals: [total(1, '90000', 7500), total(2, '4000', 10000)]
        },
        { category_account_id: 3, unassigned: false, totals: [total(1, '30000', 2500)] }
      ])
    );

    expect(rows.map((row) => row.key)).toEqual(['category:7:1', 'category:7:2', 'category:3:1']);
  });

  it('marks which rows are in the commodity the ranking was computed in', () => {
    const rows = spendingRows(
      report([
        {
          category_account_id: 7,
          unassigned: false,
          totals: [total(1, '90000', 7500), total(2, '4000', 10000)]
        }
      ])
    );

    expect(rows.map((row) => row.isRankCommodity)).toEqual([true, false]);
  });

  it('gives the unassigned bucket a stable key distinct from any category', () => {
    const rows = spendingRows(report([{ unassigned: true, totals: [total(1, '20000', 1667)] }]));

    expect(rows[0].key).toBe('unassigned:1');
  });

  it('keys payee groups separately from category groups with the same id', () => {
    const rows = spendingRows(
      report([{ payee_id: 7, payee_label: 'Corner Shop', unassigned: false, totals: [total(1, '90000', 7500)] }])
    );

    expect(rows[0].key).toBe('payee:7:1');
  });

  it('returns no rows for an empty report', () => {
    expect(spendingRows(report([], { commodity_totals: [] }))).toEqual([]);
  });
});

describe('spendingIsMultiCommodity', () => {
  it('is false for a single-commodity report', () => {
    expect(spendingIsMultiCommodity(report([]))).toBe(false);
  });

  it('is true once a second commodity appears', () => {
    const multi = report([], {
      commodity_totals: [
        { commodity_id: 1, quantity_value: '1', quantity_scale: 2, normal_quantity_value: '1' },
        { commodity_id: 2, quantity_value: '1', quantity_scale: 2, normal_quantity_value: '1' }
      ]
    });

    expect(spendingIsMultiCommodity(multi)).toBe(true);
  });
});

describe('formatShare', () => {
  it.each([
    ['a whole percentage', 7500, '75.0'],
    ['the full total', 10000, '100.0'],
    ['zero', 0, '0.0'],
    ['a tenth of a percent', 10, '0.1'],
    // 7525 basis points = 75.25%, which rounds up at the tenths place.
    ['a half-tenth rounding up', 7525, '75.3'],
    ['a refund share', -2500, '-25.0']
  ])('formats %s', (_name, basisPoints, expected) => {
    expect(formatShare(basisPoints, 'en-US')).toBe(expected);
  });

  it('keeps the sign on a negative share too small to show', () => {
    // -2 basis points is -0.02%, which rounds to 0.0 — dropping the sign would
    // present a refund as if it were nothing rather than a small inflow.
    expect(formatShare(-2, 'en-US')).toBe('-0.0');
  });
});
