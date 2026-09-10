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
  source_filename?: string;
  headers?: string[];
}

export interface CSVProfileCandidate {
  id: number;
  config: string;
}

export interface RankedCSVProfile<T extends CSVProfileCandidate> {
  profile: T;
  score: number;
}

export function parseCSVProfileConfig(raw: string): CSVProfileConfig | null {
  try {
    const value = JSON.parse(raw) as Partial<CSVProfileConfig>;
    const hasAmount = typeof value.amount_column === 'string' && value.amount_column.trim() !== '';
    const hasDebitCredit =
      typeof value.debit_column === 'string' && value.debit_column.trim() !== '' &&
      typeof value.credit_column === 'string' && value.credit_column.trim() !== '';
    if (
      !['comma', 'semicolon', 'tab'].includes(value.delimiter ?? '') ||
      typeof value.date_column !== 'string' ||
      !['DMY', 'MDY', 'YMD'].includes(value.date_layout ?? '') ||
      !['.', ','].includes(value.decimal_separator ?? '') ||
      (value.invert_amount !== undefined && typeof value.invert_amount !== 'boolean') ||
      hasAmount === hasDebitCredit
    ) return null;
    return { ...value, invert_amount: value.invert_amount ?? false } as CSVProfileConfig;
  } catch {
    return null;
  }
}

export function rankCSVProfiles<T extends CSVProfileCandidate>(
  profiles: T[],
  filename: string,
  delimiter: CSVDelimiter,
  headers: string[]
): RankedCSVProfile<T>[] {
  const headerSet = new Set(headers);
  const normalizedFilename = normalizeCSVFilename(filename);
  return profiles.flatMap((profile) => {
    const config = parseCSVProfileConfig(profile.config);
    if (!config || config.delimiter !== delimiter) return [];
    const mappedHeaders = requiredMappedHeaders(config);
    if (mappedHeaders.some((header) => !headerSet.has(header))) return [];
    let score = 20;
    if (config.headers?.length === headers.length && config.headers.every((header, index) => header === headers[index])) score += 40;
    if (config.source_filename && normalizeCSVFilename(config.source_filename) === normalizedFilename) score += 30;
    return [{ profile, score }];
  }).sort((left, right) => right.score - left.score || left.profile.id - right.profile.id);
}

export function uniqueCSVProfileSuggestion<T extends CSVProfileCandidate>(ranked: RankedCSVProfile<T>[]): T | null {
  if (ranked.length === 0 || (ranked.length > 1 && ranked[0].score === ranked[1].score)) return null;
  return ranked[0].profile;
}

function requiredMappedHeaders(config: CSVProfileConfig): string[] {
  return [
    config.date_column,
    config.amount_column,
    config.debit_column,
    config.credit_column,
    config.payee_column,
    config.memo_column,
    config.category_column,
    config.external_ref_column
  ].filter((value): value is string => !!value);
}

function normalizeCSVFilename(filename: string): string {
  return filename
    .split(/[\\/]/).at(-1)!
    .replace(/\.csv$/i, '')
    .toLowerCase()
    .replace(/\d+/g, '#')
    .replace(/[\s._-]+/g, ' ')
    .trim();
}
