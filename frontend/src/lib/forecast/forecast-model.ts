import type { components } from '$lib/api/schema';

export type ForecastSpendingModel = 'off' | 'adaptive_v1';
export type ForecastLearningPattern = 'daily' | 'weekly' | 'monthly' | 'annual_seasonal';

export const forecastLearningPatterns: ForecastLearningPattern[] = ['daily', 'weekly', 'monthly', 'annual_seasonal'];
export const forecastLearningMaxCategories = 20;

export type ForecastFilters = {
  horizonDays: number;
  accountIDs: number[];
  includeDescendants: boolean;
  reportingCurrencyID: number | null;
  spendingModel: ForecastSpendingModel;
  historyCompleteFrom: string | null;
  expenseCategoryIDs: number[];
  expensePatterns: Record<number, ForecastLearningPattern>;
};

export type ForecastFilterParseResult =
  | { valid: true; filters: ForecastFilters }
  | { valid: false; filters: ForecastFilters; reason: 'invalid_url' };

export type ForecastSeries = components['schemas']['ForecastAccountSeries'] | components['schemas']['ForecastCurrencySeries'];
export type ForecastLearnedSeries = components['schemas']['ForecastLearnedSeries'];
export type ForecastLearnedSpending = components['schemas']['ForecastLearnedSpending'];

export type ForecastChartPoint = {
  date: string;
  x: number;
  recordedY: number;
  projectedY: number;
  /** Present only when the owner opted in to estimated spending. */
  estimatedY?: number;
};

export const defaultForecastFilters: ForecastFilters = {
  horizonDays: 90,
  accountIDs: [],
  includeDescendants: true,
  reportingCurrencyID: null,
  spendingModel: 'off',
  historyCompleteFrom: null,
  expenseCategoryIDs: [],
  expensePatterns: {}
};

function scalar(params: URLSearchParams, key: string): string | null | undefined {
  const values = params.getAll(key);
  if (values.length === 0) return undefined;
  if (values.length !== 1 || values[0] === '') return null;
  return values[0];
}

export function parseForecastFilters(params: URLSearchParams): ForecastFilterParseResult {
  const fallback = { ...defaultForecastFilters, accountIDs: [] };
  const known = new Set([
    'horizon_days', 'account_id', 'include_descendants', 'reporting_currency_id', 'fx_method',
    'spending_model', 'history_complete_from', 'expense_category_id', 'expense_pattern'
  ]);
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

  const learning = parseForecastLearningFilters(params);
  if (!learning) return { valid: false, filters: fallback, reason: 'invalid_url' };

  return {
    valid: true,
    filters: {
      horizonDays,
      accountIDs: [...new Set(accountIDs)].sort((a, b) => a - b),
      includeDescendants: includeRaw === undefined ? true : includeRaw === 'true',
      reportingCurrencyID,
      ...learning
    }
  };
}

type ForecastLearningFilters = Pick<ForecastFilters, 'spendingModel' | 'historyCompleteFrom' | 'expenseCategoryIDs' | 'expensePatterns'>;

/**
 * Mirrors the backend contract: with the model off every other model parameter
 * is an orphan, so a URL carrying one is invalid rather than quietly ignored.
 */
function parseForecastLearningFilters(params: URLSearchParams): ForecastLearningFilters | null {
  const modelRaw = scalar(params, 'spending_model');
  if (modelRaw === null || (modelRaw !== undefined && modelRaw !== 'off' && modelRaw !== 'adaptive_v1')) return null;
  const historyRaw = scalar(params, 'history_complete_from');
  if (historyRaw === null) return null;
  const categoryRaw = params.getAll('expense_category_id');
  const patternRaw = params.getAll('expense_pattern');
  const model: ForecastSpendingModel = modelRaw === 'adaptive_v1' ? 'adaptive_v1' : 'off';

  if (model === 'off') {
    if (historyRaw !== undefined || categoryRaw.length > 0 || patternRaw.length > 0) return null;
    return { spendingModel: 'off', historyCompleteFrom: null, expenseCategoryIDs: [], expensePatterns: {} };
  }
  if (historyRaw === undefined || !/^\d{4}-\d{2}-\d{2}$/.test(historyRaw) || Number.isNaN(Date.parse(historyRaw))) return null;

  const expenseCategoryIDs: number[] = [];
  for (const raw of categoryRaw) {
    const id = Number(raw);
    if (!/^\d+$/.test(raw) || !Number.isSafeInteger(id) || id <= 0) return null;
    expenseCategoryIDs.push(id);
  }
  const categories = [...new Set(expenseCategoryIDs)].sort((a, b) => a - b);
  if (categories.length > forecastLearningMaxCategories) return null;

  const expensePatterns: Record<number, ForecastLearningPattern> = {};
  for (const raw of patternRaw) {
    // Split at the final colon, exactly as the backend does.
    const index = raw.lastIndexOf(':');
    if (index <= 0 || index === raw.length - 1) return null;
    const id = Number(raw.slice(0, index));
    const pattern = raw.slice(index + 1) as ForecastLearningPattern;
    if (!/^\d+$/.test(raw.slice(0, index)) || !Number.isSafeInteger(id) || id <= 0) return null;
    if (!forecastLearningPatterns.includes(pattern)) return null;
    if (expensePatterns[id] !== undefined) return null;
    if (categories.length > 0 && !categories.includes(id)) return null;
    expensePatterns[id] = pattern;
  }
  if (Object.keys(expensePatterns).length > forecastLearningMaxCategories) return null;

  return { spendingModel: 'adaptive_v1', historyCompleteFrom: historyRaw, expenseCategoryIDs: categories, expensePatterns };
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
  if (filters.spendingModel === 'adaptive_v1' && filters.historyCompleteFrom) {
    params.set('spending_model', 'adaptive_v1');
    params.set('history_complete_from', filters.historyCompleteFrom);
    const categories = [...new Set(filters.expenseCategoryIDs)].sort((a, b) => a - b);
    for (const id of categories) params.append('expense_category_id', String(id));
    const overrides = Object.entries(filters.expensePatterns)
      .map(([id, pattern]) => [Number(id), pattern] as const)
      .filter(([id]) => categories.length === 0 || categories.includes(id))
      .sort((a, b) => a[0] - b[0]);
    for (const [id, pattern] of overrides) params.append('expense_pattern', `${id}:${pattern}`);
  }
  return params;
}

/** Finds the learned curve matching a core series, if the overlay has one. */
export function forecastLearnedSeriesFor(
  learned: ForecastLearnedSpending | null | undefined,
  accountID: number | undefined,
  commodityID: number,
  converted = false
): ForecastLearnedSeries | undefined {
  if (!learned || learned.status === 'unavailable') return undefined;
  if (converted) return learned.converted ?? undefined;
  const rows = accountID === undefined ? learned.totals : learned.series;
  return rows.find((row) => row.commodity_id === commodityID && (accountID === undefined || row.account_id === accountID));
}

export function forecastHasMovements(series: ForecastSeries[]): boolean {
  return series.some((row) => row.points.some((point) =>
    point.posted_delta.quantity_value !== '0' || point.draft_delta.quantity_value !== '0' || point.template_delta.quantity_value !== '0'
  ));
}

function scaledValue(value: string, scale: number, targetScale: number): bigint {
  return BigInt(value) * 10n ** BigInt(targetScale - scale);
}

export function forecastChartPoints(series: ForecastSeries, estimated?: ForecastLearnedSeries): ForecastChartPoint[] {
  if (series.points.length === 0) return [];
  const estimatedByDate = new Map((estimated?.points ?? []).map((point) => [point.date, point.projected_balance]));
  const scale = series.points.reduce((maximum, point) => Math.max(
    maximum,
    point.recorded_balance.quantity_scale,
    point.projected_balance.quantity_scale,
    estimatedByDate.get(point.date)?.quantity_scale ?? 0
  ), 0);
  const values = series.points.flatMap((point) => {
    const row = [
      scaledValue(point.recorded_balance.quantity_value, point.recorded_balance.quantity_scale, scale),
      scaledValue(point.projected_balance.quantity_value, point.projected_balance.quantity_scale, scale)
    ];
    const estimate = estimatedByDate.get(point.date);
    if (estimate) row.push(scaledValue(estimate.quantity_value, estimate.quantity_scale, scale));
    return row;
  });
  const minimum = values.reduce((result, value) => value < result ? value : result, 0n);
  const maximum = values.reduce((result, value) => value > result ? value : result, 0n);
  const span = maximum - minimum;
  const ratio = (value: bigint) => span === 0n ? 0.5 : Number(((maximum - value) * 1_000_000n) / span) / 1_000_000;
  const divisor = Math.max(series.points.length - 1, 1);
  return series.points.map((point, index) => {
    const estimate = estimatedByDate.get(point.date);
    return {
      date: point.date,
      x: index / divisor,
      recordedY: ratio(scaledValue(point.recorded_balance.quantity_value, point.recorded_balance.quantity_scale, scale)),
      projectedY: ratio(scaledValue(point.projected_balance.quantity_value, point.projected_balance.quantity_scale, scale)),
      estimatedY: estimate ? ratio(scaledValue(estimate.quantity_value, estimate.quantity_scale, scale)) : undefined
    };
  });
}
