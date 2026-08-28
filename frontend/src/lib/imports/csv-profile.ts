export type CSVDelimiter = 'comma' | 'semicolon' | 'tab';

export interface CSVProfileConfig {
  delimiter: CSVDelimiter;
  date_column: string;
  payee_column?: string;
  memo_column?: string;
  category_column?: string;
  external_ref_column?: string;
  amount_column?: string;
  debit_column?: string;
  credit_column?: string;
  date_layout: 'DMY' | 'MDY' | 'YMD';
  decimal_separator: '.' | ',';
  invert_amount: boolean;
}

export function delimiterCharacter(delimiter: CSVDelimiter): string {
  return delimiter === 'semicolon' ? ';' : delimiter === 'tab' ? '\t' : ',';
}

export function detectCSVDelimiter(line: string): CSVDelimiter {
  const candidates: CSVDelimiter[] = ['comma', 'semicolon', 'tab'];
  return candidates.reduce((best, candidate) =>
    countOutsideQuotes(line, delimiterCharacter(candidate)) > countOutsideQuotes(line, delimiterCharacter(best))
      ? candidate
      : best
  );
}

export function parseCSVHeader(text: string, delimiter?: CSVDelimiter): { delimiter: CSVDelimiter; headers: string[] } {
  const firstLine = text.replace(/^\uFEFF/, '').split(/\r?\n/, 1)[0] ?? '';
  const resolved = delimiter ?? detectCSVDelimiter(firstLine);
  const headers = parseCSVLine(firstLine, delimiterCharacter(resolved)).map((value) => value.trim());
  if (headers.length < 2 || headers.some((value) => !value) || new Set(headers).size !== headers.length) {
    throw new Error('invalid CSV header');
  }
  return { delimiter: resolved, headers };
}

function countOutsideQuotes(line: string, delimiter: string): number {
  let count = 0;
  let quoted = false;
  for (let index = 0; index < line.length; index += 1) {
    if (line[index] === '"') {
      if (quoted && line[index + 1] === '"') index += 1;
      else quoted = !quoted;
    } else if (!quoted && line[index] === delimiter) count += 1;
  }
  return count;
}

function parseCSVLine(line: string, delimiter: string): string[] {
  const values: string[] = [];
  let value = '';
  let quoted = false;
  for (let index = 0; index < line.length; index += 1) {
    const char = line[index];
    if (char === '"') {
      if (quoted && line[index + 1] === '"') {
        value += '"';
        index += 1;
      } else quoted = !quoted;
    } else if (char === delimiter && !quoted) {
      values.push(value);
      value = '';
    } else value += char;
  }
  if (quoted) throw new Error('invalid CSV header');
  values.push(value);
  return values;
}
