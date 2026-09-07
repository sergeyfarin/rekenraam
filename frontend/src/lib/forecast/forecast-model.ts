import type { components } from '$lib/api/schema';

export type ForecastFilters = {
  horizonDays: number;
  accountIDs: number[];
  includeDescendants: boolean;
  reportingCurrencyID: number | null;
};

export type ForecastFilterParseResult =
  | { valid: true; filters: ForecastFilters }
  | { valid: false; filters: ForecastFilters; reason: 'invalid_url' };

export type ForecastSeries = components['schemas']['ForecastAccountSeries'] | components['schemas']['ForecastCurrencySeries'];

export type ForecastChartPoint = {
  date: string;
  x: number;
  recordedY: number;
  projectedY: number;
};

export const defaultForecastFilters: ForecastFilters = {
  horizonDays: 90,
  accountIDs: [],
  includeDescendants: true,
  reportingCurrencyID: null
};

function scalar(params: URLSearchParams, key: string): string | null | undefined {
  const values = params.getAll(key);
  if (values.length === 0) return undefined;
  if (values.length !== 1 || values[0] === '') return null;
  return values[0];
}

export function parseForecastFilters(params: URLSearchParams): ForecastFilterParseResult {
  const fallback = { ...defaultForecastFilters, accountIDs: [] };
  const known = new Set(['horizon_days', 'account_id', 'include_descendants', 'reporting_currency_id', 'fx_method']);
  if ([...params.keys()].some((key) => !known.has(key))) return { valid: false, filters: fallback, reason: 'invalid_url' };

  const horizonRaw = scalar(params, 'horizon_days');
  const horizonDays = horizonRaw === undefined ? 90 : Number(horizonRaw);
  if (horizonRaw === null || !Number.isInteger(horizonDays) || horizonDays < 1 || horizonDays > 366) {
    return { valid: false, filters: fallback, reason: 'invalid_url' };
  }

  const includeRaw = scalar(params, 'include_descendants');
  if (includeRaw === null || (includeRaw !== undefined && includeRaw !== 'true' && includeRaw !== 'false')) {
    return { valid: false, filters: fallback, reason: 'invalid_url' };
  }

  const accountIDs: number[] = [];
  for (const raw of params.getAll('account_id')) {
    const id = Number(raw);
    if (!/^\d+$/.test(raw) || !Number.isSafeInteger(id) || id <= 0) {
      return { valid: false, filters: fallback, reason: 'invalid_url' };
    }
    accountIDs.push(id);
  }

  const reportingRaw = scalar(params, 'reporting_currency_id');
  const method = scalar(params, 'fx_method');
  if (reportingRaw === null || method === null || (reportingRaw === undefined) !== (method === undefined) || (method !== undefined && method !== 'constant_as_of')) {
    return { valid: false, filters: fallback, reason: 'invalid_url' };
  }
  let reportingCurrencyID: number | null = null;
  if (reportingRaw !== undefined) {
    reportingCurrencyID = Number(reportingRaw);
    if (!/^\d+$/.test(reportingRaw) || !Number.isSafeInteger(reportingCurrencyID) || reportingCurrencyID <= 0) {
      return { valid: false, filters: fallback, reason: 'invalid_url' };
    }
  }

  return {
    valid: true,
    filters: {
      horizonDays,
      accountIDs: [...new Set(accountIDs)].sort((a, b) => a - b),
      includeDescendants: includeRaw === undefined ? true : includeRaw === 'true',
      reportingCurrencyID
    }
  };
}

export function writeForecastFilters(filters: ForecastFilters): URLSearchParams {
  const params = new URLSearchParams();
  params.set('horizon_days', String(filters.horizonDays));
  for (const id of [...new Set(filters.accountIDs)].sort((a, b) => a - b)) params.append('account_id', String(id));
  if (filters.accountIDs.length > 0) params.set('include_descendants', String(filters.includeDescendants));
  if (filters.reportingCurrencyID !== null) {
    params.set('reporting_currency_id', String(filters.reportingCurrencyID));
    params.set('fx_method', 'constant_as_of');
  }
  return params;
}

export function forecastHasMovements(series: ForecastSeries[]): boolean {
  return series.some((row) => row.points.some((point) =>
    point.posted_delta.quantity_value !== '0' || point.draft_delta.quantity_value !== '0' || point.template_delta.quantity_value !== '0'
  ));
}

function scaledValue(value: string, scale: number, targetScale: number): bigint {
  return BigInt(value) * 10n ** BigInt(targetScale - scale);
}

export function forecastChartPoints(series: ForecastSeries): ForecastChartPoint[] {
  if (series.points.length === 0) return [];
  const scale = series.points.reduce((maximum, point) => Math.max(maximum, point.recorded_balance.quantity_scale, point.projected_balance.quantity_scale), 0);
  const values = series.points.flatMap((point) => [
    scaledValue(point.recorded_balance.quantity_value, point.recorded_balance.quantity_scale, scale),
    scaledValue(point.projected_balance.quantity_value, point.projected_balance.quantity_scale, scale)
  ]);
  const minimum = values.reduce((result, value) => value < result ? value : result, 0n);
  const maximum = values.reduce((result, value) => value > result ? value : result, 0n);
  const span = maximum - minimum;
  const ratio = (value: bigint) => span === 0n ? 0.5 : Number(((maximum - value) * 1_000_000n) / span) / 1_000_000;
  const divisor = Math.max(series.points.length - 1, 1);
  return series.points.map((point, index) => ({
    date: point.date,
    x: index / divisor,
    recordedY: ratio(scaledValue(point.recorded_balance.quantity_value, point.recorded_balance.quantity_scale, scale)),
    projectedY: ratio(scaledValue(point.projected_balance.quantity_value, point.projected_balance.quantity_scale, scale))
  }));
}
