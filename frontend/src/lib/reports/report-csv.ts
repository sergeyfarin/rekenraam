import type { CashflowResponse, NetWorthSeriesResponse, SpendingResponse } from '$lib/api/ledger';
import { formatLedgerAmount } from '$lib/money/amount';
import { cashflowRows } from './cashflow';
import { netWorthRows } from './net-worth';
import { spendingRows } from './spending';

/**
 * CSV export for the three R2 reports.
 *
 * ## Why amounts use `formatLedgerAmount`, not `formatQuantity`
 *
 * A CSV is read by a spreadsheet, not by a person. `formatQuantity` produces
 * locale group separators ("1,234.56"), which a spreadsheet either splits
 * across columns or refuses to parse as a number. `formatLedgerAmount` is the
 * plain-decimal half of `$lib/money` — the same output an editable input round-
 * trips through — and that is exactly the shape a machine reader wants.
 *
 * This is the convention in `docs/conventions.md` doing its job: the choice is
 * between two named functions, so the export cannot quietly grow a third
 * formatting rule of its own. No arithmetic happens in this module at all;
 * every figure is already exact and final from the backend.
 *
 * Dates stay ISO 8601 and commodity columns carry the code, so a file opened
 * six months later is still unambiguous.
 */

/** RFC 4180 quoting: wrap when needed, and double any embedded quote. */
function csvField(value: string): string {
  if (value === '') return '';
  if (/[",\r\n]/.test(value)) {
    return `"${value.replace(/"/g, '""')}"`;
  }
  return value;
}

export function toCSV(rows: string[][]): string {
  // CRLF line endings, per RFC 4180 — Excel is the least forgiving consumer.
  return rows.map((row) => row.map(csvField).join(',')).join('\r\n');
}

/**
 * A header block naming the query that produced the file.
 *
 * The plan requires that an exported report still be understandable later, and
 * a bare table of numbers is not: without the range and the exclusion policy, a
 * reader cannot tell whether transfers were counted.
 */
function queryHeader(lines: string[][]): string[][] {
  return [...lines, []];
}

export function netWorthCSV(report: NetWorthSeriesResponse, commodityCode: (id: number) => string): string {
  const rows = queryHeader([
    ['Report', 'Net worth over time'],
    ['Start date', report.start_date],
    ['End date', report.end_date],
    ['Bucket', report.bucket],
    ['Basis', 'Posted transactions only'],
    ['Excluded system roles', report.excluded_system_roles.join('; ')]
  ]);

  rows.push(['Period start', 'Period end', 'Commodity', 'Net worth']);
  for (const row of netWorthRows(report)) {
    rows.push([
      row.startDate,
      row.endDate,
      commodityCode(row.commodity_id),
      formatLedgerAmount(row.normal_quantity_value, row.quantity_scale)
    ]);
  }
  return toCSV(rows);
}

export function spendingCSV(
  report: SpendingResponse,
  commodityCode: (id: number) => string,
  groupLabel: (row: ReturnType<typeof spendingRows>[number]) => string
): string {
  const rows = queryHeader([
    ['Report', report.direction === 'income' ? 'Income by dimension' : 'Spending by dimension'],
    ['Start date', report.start_date],
    ['End date', report.end_date],
    ['Grouped by', report.group_by],
    ['Direction', report.direction],
    ['Basis', 'Posted transactions only'],
    ['Excluded', 'Transfers between own accounts; system accounts'],
    ['Excluded system roles', report.excluded_system_roles.join('; ')],
    ['Ranked by commodity', commodityCode(report.rank_commodity_id)]
  ]);

  // The share column is labelled as within-commodity so a reader cannot add up
  // percentages across two currencies and think the result means something.
  rows.push([report.group_by === 'payee' ? 'Payee' : 'Category', 'Commodity', 'Amount', 'Share of commodity total (%)']);
  for (const row of spendingRows(report)) {
    rows.push([
      groupLabel(row),
      commodityCode(row.commodityId),
      formatLedgerAmount(row.normalQuantityValue, row.quantityScale),
      // Basis points to a percentage, as exact integer string work.
      formatLedgerAmount(String(row.shareBasisPoints), 2)
    ]);
  }

  rows.push([]);
  rows.push(['Total', '', '', '']);
  for (const total of report.commodity_totals) {
    rows.push([
      'Total',
      commodityCode(total.commodity_id),
      formatLedgerAmount(total.normal_quantity_value, total.quantity_scale),
      ''
    ]);
  }
  return toCSV(rows);
}

export function cashflowCSV(report: CashflowResponse, commodityCode: (id: number) => string): string {
  const rows = queryHeader([
    ['Report', 'Cashflow'],
    ['Start date', report.start_date],
    ['End date', report.end_date],
    ['Bucket', report.bucket],
    ['Basis', 'Posted transactions only'],
    ['Cash scope kinds', report.scope_kinds.join('; ')],
    [
      'Cash scope accounts',
      report.scope_accounts.map((account) => account.name || account.code || `#${account.account_id}`).join('; ')
    ],
    ['Transfer policy', 'Movement between two in-scope cash accounts is excluded; movement to anything else is a transfer'],
    ['Excluded system roles', report.excluded_system_roles.join('; ')]
  ]);

  rows.push([
    'Period start',
    'Period end',
    'Commodity',
    'Inflow',
    'Outflow',
    'Operating net',
    'Transfer in',
    'Transfer out',
    'Net movement'
  ]);
  for (const row of cashflowRows(report)) {
    if (row.commodityId === null) {
      // An empty bucket is exported as an empty row rather than skipped, so the
      // periods in the file match the periods on screen.
      rows.push([row.startDate, row.endDate, '', '', '', '', '', '', '']);
      continue;
    }
    rows.push([
      row.startDate,
      row.endDate,
      commodityCode(row.commodityId),
      formatLedgerAmount(row.inflow, row.quantityScale),
      formatLedgerAmount(row.outflow, row.quantityScale),
      formatLedgerAmount(row.operatingNet, row.quantityScale),
      formatLedgerAmount(row.transferIn, row.quantityScale),
      formatLedgerAmount(row.transferOut, row.quantityScale),
      formatLedgerAmount(row.netMovement, row.quantityScale)
    ]);
  }
  return toCSV(rows);
}

/**
 * Triggers a browser download of a CSV string.
 *
 * A UTF-8 BOM is prepended because Excel otherwise reads a UTF-8 CSV as the
 * system codepage and mangles any non-ASCII payee or account name.
 */
export function downloadCSV(filename: string, csv: string): void {
  const blob = new Blob(['﻿', csv], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
}

/** A filename that carries the report and its range, e.g. `cashflow-2026-01-01-2026-06-30.csv`. */
export function reportFilename(report: string, startDate: string, endDate: string): string {
  return `${report}-${startDate}-${endDate}.csv`;
}
