import { describe, expect, it } from 'vitest';
import type { NetWorthSeriesResponse } from '$lib/api/ledger';
import { hasMultipleCommodities, netWorthRows } from './net-worth';

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
        endDate: '2026-06-07'
      },
      {
        commodity_id: 1,
        quantity_value: '12500',
        quantity_scale: 2,
        normal_quantity_value: '12500',
        startDate: '2026-06-08',
        endDate: '2026-06-14'
      },
      {
        commodity_id: 2,
        quantity_value: '3000',
        quantity_scale: 2,
        normal_quantity_value: '3000',
        startDate: '2026-06-08',
        endDate: '2026-06-14'
      }
    ]);
  });

  it('flags when totals cannot be combined across commodities', () => {
    expect(hasMultipleCommodities(netWorthRows(report))).toBe(true);
    expect(hasMultipleCommodities(netWorthRows({ ...report, buckets: [report.buckets[0]] }))).toBe(false);
  });
});
