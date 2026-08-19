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
 */

/** Renders an exact coefficient and scale as a plain decimal string. */
export function exactDecimal(quantityValue: string, quantityScale: number): string {
  const scale = Number.isInteger(quantityScale) && quantityScale > 0 ? quantityScale : 0;

  let coefficient: bigint;
  try {
    coefficient = BigInt(quantityValue);
  } catch {
    // A coefficient the backend could not have produced is passed through
    // rather than guessed at: an unreadable cell is better than a wrong number.
    return quantityValue;
  }

  const negative = coefficient < 0n;
  const digits = (negative ? -coefficient : coefficient).toString();
  if (scale === 0) {
    return `${negative ? '-' : ''}${digits}`;
  }

  const padded = digits.padStart(scale + 1, '0');
  const whole = padded.slice(0, padded.length - scale);
  const fraction = padded.slice(padded.length - scale);
  return `${negative ? '-' : ''}${whole}.${fraction}`;
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
