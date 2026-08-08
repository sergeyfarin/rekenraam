import { describe, it, expect } from 'vitest';
import type { CashflowResponse, NetWorthSeriesResponse, SpendingResponse } from '$lib/api/ledger';
import { cashflowCSV, netWorthCSV, reportFilename, spendingCSV, toCSV } from './report-csv';

const code = (id: number) => (id === 1 ? 'EUR' : `#${id}`);

function lines(csv: string): string[] {
  return csv.split('\r\n');
}

function dataRows(csv: string, headerText: string): string[] {
  const all = lines(csv);
  const start = all.findIndex((line) => line.startsWith(headerText));
  return all.slice(start + 1).filter((line) => line !== '');
}

describe('toCSV', () => {
  it.each([
    ['a plain value', 'Groceries', 'Groceries'],
    ['a value containing a comma', 'Shop, The', '"Shop, The"'],
    ['a value containing a quote', 'The "Shop"', '"The ""Shop"""'],
    ['a value containing a newline', 'Line1\nLine2', '"Line1\nLine2"'],
    ['an empty value', '', '']
  ])('quotes %s per RFC 4180', (_name, value, expected) => {
    expect(toCSV([[value]])).toBe(expected);
  });

  it('separates rows with CRLF, which Excel requires', () => {
    expect(toCSV([['a'], ['b']])).toBe('a\r\nb');
  });
});

describe('netWorthCSV', () => {
  const report: NetWorthSeriesResponse = {
    start_date: '2026-01-01',
    end_date: '2026-02-28',
    bucket: 'month',
    query: { start_date: '2026-01-01', end_date: '2026-02-28', bucket: 'month' },
    buckets: [
      {
        start_date: '2026-01-01',
        end_date: '2026-01-31',
        totals: [
          { commodity_id: 1, quantity_value: '123456789', quantity_scale: 2, normal_quantity_value: '123456789' }
        ]
      }
    ],
    excluded_system_roles: ['commodity_trading']
  } as NetWorthSeriesResponse;

  it('writes amounts as plain decimals a spreadsheet can parse', () => {
    // formatQuantity would emit "1,234,567.89", which a spreadsheet splits
    // across columns. This is why the export uses formatLedgerAmount.
    expect(dataRows(netWorthCSV(report, code), 'Period start')[0]).toBe(
      '2026-01-01,2026-01-31,EUR,1234567.89'
    );
  });

  it('names the query and the exclusion policy above the table', () => {
    const csv = netWorthCSV(report, code);

    expect(csv).toContain('Start date,2026-01-01');
    expect(csv).toContain('End date,2026-02-28');
    expect(csv).toContain('Basis,Posted transactions only');
    expect(csv).toContain('Excluded system roles,commodity_trading');
  });
});

describe('spendingCSV', () => {
  const report: SpendingResponse = {
    start_date: '2026-06-01',
    end_date: '2026-06-30',
    group_by: 'payee',
    direction: 'expense',
    rank_commodity_id: 1,
    query: { start_date: '2026-06-01', end_date: '2026-06-30', group_by: 'payee', direction: 'expense' },
    commodity_totals: [
      { commodity_id: 1, quantity_value: '120000', quantity_scale: 2, normal_quantity_value: '120000' }
    ],
    groups: [
      {
        payee_id: 4,
        payee_label: 'Shop, The',
        unassigned: false,
        totals: [
          {
            commodity_id: 1,
            quantity_value: '90000',
            quantity_scale: 2,
            normal_quantity_value: '90000',
            share_basis_points: 7500
          }
        ]
      }
    ],
    excluded_system_roles: ['commodity_trading']
  } as SpendingResponse;

  const label = (row: { group: { payee_label?: string } }) => row.group.payee_label ?? '';

  it('quotes a payee name containing a comma rather than splitting the row', () => {
    expect(dataRows(spendingCSV(report, code, label), 'Payee,Commodity')[0]).toBe(
      '"Shop, The",EUR,900.00,75.00'
    );
  });

  it('renders the share as a percentage from exact basis points', () => {
    // 7500 basis points is 75.00%, produced by string work rather than by
    // dividing through a float.
    expect(spendingCSV(report, code, label)).toContain(',75.00');
  });

  it('labels the share column as within-commodity', () => {
    // Or a reader would add percentages across two currencies and think the
    // sum meant something.
    expect(spendingCSV(report, code, label)).toContain('Share of commodity total (%)');
  });

  it('states that transfers were excluded', () => {
    expect(spendingCSV(report, code, label)).toContain('Excluded,Transfers between own accounts; system accounts');
  });

  it('names the commodity the ranking used', () => {
    expect(spendingCSV(report, code, label)).toContain('Ranked by commodity,EUR');
  });

  it('appends per-commodity totals', () => {
    expect(spendingCSV(report, code, label)).toContain('Total,EUR,1200.00,');
  });
});

describe('cashflowCSV', () => {
  const report: CashflowResponse = {
    start_date: '2026-06-01',
    end_date: '2026-07-31',
    bucket: 'month',
    query: { start_date: '2026-06-01', end_date: '2026-07-31', bucket: 'month' },
    scope_kinds: ['cash', 'checking'],
    scope_accounts: [{ account_id: 1, name: 'Checking', account_kind: 'checking' }],
    buckets: [
      {
        start_date: '2026-06-01',
        end_date: '2026-06-30',
        totals: [
          {
            commodity_id: 1,
            quantity_scale: 2,
            inflow: '200000',
            outflow: '90000',
            operating_net: '110000',
            transfer_in: '0',
            transfer_out: '40000',
            net_movement: '70000'
          }
        ]
      },
      { start_date: '2026-07-01', end_date: '2026-07-31', totals: [] }
    ],
    excluded_system_roles: ['commodity_trading']
  } as CashflowResponse;

  it('writes every classified figure as a plain decimal', () => {
    expect(dataRows(cashflowCSV(report, code), 'Period start')[0]).toBe(
      '2026-06-01,2026-06-30,EUR,2000.00,900.00,1100.00,0.00,400.00,700.00'
    );
  });

  it('keeps a bucket with no movement as an empty row so periods match the screen', () => {
    expect(dataRows(cashflowCSV(report, code), 'Period start')[1]).toBe('2026-07-01,2026-07-31,,,,,,,');
  });

  it('records the cash scope and the transfer policy', () => {
    const csv = cashflowCSV(report, code);

    expect(csv).toContain('Cash scope kinds,cash; checking');
    expect(csv).toContain('Cash scope accounts,Checking');
    expect(csv).toContain('Transfer policy,');
  });
});

describe('reportFilename', () => {
  it('carries the report name and its range', () => {
    expect(reportFilename('cashflow', '2026-01-01', '2026-06-30')).toBe('cashflow-2026-01-01-2026-06-30.csv');
  });
});
