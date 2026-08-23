import { formatLedgerAmount } from '$lib/money/amount';

/**
 * CSV export of what a report screen is showing.
 *
 * Two rules make the difference between a file a spreadsheet can read and one
 * it silently corrupts:
 *
 * - **Amounts are exact and unformatted.** A locale-formatted figure carries
 *   grouping separators, and in most of Europe a comma decimal separator — both
 *   destroy a CSV. Values go out as the canonical coefficient scaled to a plain
 *   decimal, with the commodity in its own column.
 * - **Quoting follows RFC 4180**, so a payee named `Smith, Jones & Co "The
 *   Grocer"` survives the round trip.
 *
 * `exactDecimal` delegates the actual digit-shifting to
 * `$lib/money/amount.ts`'s `formatLedgerAmount` — the same plain-decimal
 * formatter every editable amount input round-trips through — rather than
 * reimplementing it here. Duplicated decimal-formatting helpers are a
 * documented recurring bug class in this codebase (T-36/T-45/T-47, a silent
 * 100x error from a second copy drifting out of sync); this module must not
 * grow another one.
 */

/** Renders an exact coefficient and scale as a plain decimal string. */
export function exactDecimal(quantityValue: string, quantityScale: number): string {
  const scale = Number.isInteger(quantityScale) && quantityScale > 0 ? quantityScale : 0;

  // formatLedgerAmount trusts its input to be a signed digit string and does
  // not validate it. A coefficient the backend could not have produced is
  // passed through unchanged rather than guessed at: an unreadable cell in an
  // exported file is better than a silently wrong number.
  if (!/^-?\d+$/.test(quantityValue)) {
    return quantityValue;
  }

  return formatLedgerAmount(quantityValue, scale);
}

/**
 * Appends the report's own context beneath the data table.
 *
 * A file of figures that does not say what it measured is a file nobody can
 * check later — which is the same reason cashflow states its scope on screen
 * rather than assuming it. The context goes *after* the table, so the first
 * line of the file is still the header row; a title block would move it.
 *
 * Every row, including the separator and each context line, is padded to the
 * table's column count. A strict RFC 4180 reader may reject a file whose rows
 * disagree on field count, so ragged rows would make the context the thing that
 * broke the export. Padded, the file stays rectangular: the context reads as
 * trailing records whose later fields are empty, which every reader accepts
 * even though only a spreadsheet renders it the way a person would want.
 */
export function withReportContext(rows: string[][], context: string[]): string[][] {
  const lines = context.filter((line) => line.trim() !== '');
  if (lines.length === 0) {
    return rows;
  }
  const width = rows.reduce((widest, row) => Math.max(widest, row.length), 0);
  const pad = (row: string[]) => [...row, ...Array<string>(Math.max(0, width - row.length)).fill('')];
  return [...rows.map(pad), pad([]), ...lines.map((line) => pad([line]))];
}

/** Quotes one field per RFC 4180, only where quoting is actually needed. */
export function csvField(value: string): string {
  return /[",\r\n]/.test(value) ? `"${value.replaceAll('"', '""')}"` : value;
}

export function toCSV(rows: string[][]): string {
  return rows.map((row) => row.map(csvField).join(',')).join('\r\n');
}

/** `rekenraam-spending-2026-06-01-2026-06-30.csv` */
export function csvFilename(report: string, startDate: string, endDate: string): string {
  return `rekenraam-${report}-${startDate}-${endDate}.csv`;
}

/**
 * Hands the file to the browser.
 *
 * The leading BOM is what makes Excel read the file as UTF-8 rather than as the
 * system codepage, which otherwise mangles every non-ASCII payee name.
 */
export function downloadCSV(filename: string, content: string): void {
  const blob = new Blob([`﻿${content}`], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  try {
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    link.rel = 'noopener';
    document.body.append(link);
    link.click();
    link.remove();
  } finally {
    URL.revokeObjectURL(url);
  }
}
