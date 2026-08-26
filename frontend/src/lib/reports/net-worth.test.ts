import { describe, expect, it } from 'vitest';
import type { NetWorthSeriesResponse } from '$lib/api/reports';
import { convertedSeries, hasMultipleCommodities, netWorthRows } from './net-worth';

const report: NetWorthSeriesResponse = {
  start_date: '2026-06-01',
  end_date: '2026-06-14',
  bucket: 'week',
  query: {
    start_date: '2026-06-01',
    end_date: '2026-06-14',
    bucket: 'week',
    filters: {
      account_ids: [],
      include_descendants: false,
      commodity_ids: [],
      resolved_account_ids: []
    }
  },
  buckets: [
    {
      start_date: '2026-06-01',
      end_date: '2026-06-07',
      totals: [
        {
          commodity_id: 1,
          quantity_value: '10000',
          quantity_scale: 2,
          normal_quantity_value: '10000'
        }
      ]
    },
    {
      start_date: '2026-06-08',
      end_date: '2026-06-14',
      totals: [
        {
          commodity_id: 1,
          quantity_value: '12500',
          quantity_scale: 2,
          normal_quantity_value: '12500'
        },
        {
          commodity_id: 2,
          quantity_value: '3000',
          quantity_scale: 2,
          normal_quantity_value: '3000'
        }
      ]
    }
  ],
  excluded_system_roles: ['commodity_trading']
};

describe('netWorthRows', () => {
  it('keeps each exact commodity total attached to its bucket', () => {
    expect(netWorthRows(report)).toEqual([
      {
        commodity_id: 1,
        quantity_value: '10000',
        quantity_scale: 2,
        normal_quantity_value: '10000',
        startDate: '2026-06-01',
        converted: false,
        endDate: '2026-06-07'
      },
      {
        commodity_id: 1,
        quantity_value: '12500',
        quantity_scale: 2,
        normal_quantity_value: '12500',
        startDate: '2026-06-08',
        converted: false,
        endDate: '2026-06-14'
      },
      {
        commodity_id: 2,
        quantity_value: '3000',
        quantity_scale: 2,
        normal_quantity_value: '3000',
        startDate: '2026-06-08',
        converted: false,
        endDate: '2026-06-14'
      }
    ]);
  });

  it('flags when totals cannot be combined across commodities', () => {
    expect(hasMultipleCommodities(netWorthRows(report))).toBe(true);
    expect(hasMultipleCommodities(netWorthRows({ ...report, buckets: [report.buckets[0]] }))).toBe(false);
  });
});

describe('the reporting-currency restatement', () => {
  const quantity = (value: string) => ({
    commodity_id: 9,
    quantity_value: value,
    quantity_scale: 2,
    normal_quantity_value: value
  });

  const converted = (first?: string, second?: string): NetWorthSeriesResponse => ({
    ...report,
    buckets: [
      { ...report.buckets[0], converted: first ? quantity(first) : undefined },
      { ...report.buckets[1], converted: second ? quantity(second) : undefined }
    ]
  });

  it('follows the holdings it restates rather than leading them', () => {
    const rows = netWorthRows(converted('9000', '14000'));
    expect(rows.map((row) => [row.commodity_id, row.converted])).toEqual([
      [1, false],
      [9, true],
      [1, false],
      [2, false],
      [9, true]
    ]);
  });

  it('is not a holding, so it does not make a book multi-commodity', () => {
    const rows = netWorthRows({
      ...converted('9000', '14000'),
      buckets: [{ ...report.buckets[0], converted: quantity('9000') }]
    });
    expect(hasMultipleCommodities(rows)).toBe(false);
  });

  it('charts nothing when a bucket did not convert', () => {
    // A series drawn with a hole in it would show a fall to zero that never
    // happened, which is worse than showing no chart.
    expect(convertedSeries(netWorthRows(converted('9000')))).toEqual([]);
    expect(convertedSeries(netWorthRows(converted('9000', '14000')))).toHaveLength(2);
  });

  it('charts nothing when no reporting currency was asked for', () => {
    expect(convertedSeries(netWorthRows(report))).toEqual([]);
  });
});

it('does not turn a range with no holdings into a table of zeros', () => {
  // The backend converts an empty bucket to an exact zero, which is true but is
  // not a net worth.
  const rows = netWorthRows({
    ...report,
    buckets: [
      {
        start_date: '2026-06-01',
        end_date: '2026-06-07',
        totals: [],
        converted: { commodity_id: 9, quantity_value: '0', quantity_scale: 2, normal_quantity_value: '0' }
      }
    ]
  });
  expect(rows).toEqual([]);
});
