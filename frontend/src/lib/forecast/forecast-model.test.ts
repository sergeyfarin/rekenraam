import { describe, expect, it } from 'vitest';
import {
  defaultForecastFilters,
  forecastChartPoints,
  forecastHasMovements,
  forecastLearnedSeriesFor,
  parseForecastFilters,
  writeForecastFilters,
  type ForecastLearnedSeries,
  type ForecastLearnedSpending,
  type ForecastSeries
} from './forecast-model';

function series(recorded: string[], projected = recorded): ForecastSeries {
  return {
    commodity_id: 1,
    commodity_code: 'EUR',
    opening_balance: { quantity_value: '0', quantity_scale: 2 },
    minimum_balance: { quantity_value: '0', quantity_scale: 2 },
    minimum_date: '2026-09-07',
    points: recorded.map((value, index) => ({
      date: `2026-09-${String(index + 8).padStart(2, '0')}`,
      posted_delta: { quantity_value: '0', quantity_scale: 2 },
      draft_delta: { quantity_value: '0', quantity_scale: 2 },
      template_delta: { quantity_value: '0', quantity_scale: 2 },
      recorded_balance: { quantity_value: value, quantity_scale: 2 },
      projected_balance: { quantity_value: projected[index], quantity_scale: 2 },
      source_event_counts: { posted: 0, draft: 0, template: 0 },
      carried_forward_event_count: 0
    }))
  };
}

describe('forecast filter model', () => {
  it('round-trips sorted unique accounts and explicit descendant false', () => {
    const parsed = parseForecastFilters(new URLSearchParams('horizon_days=30&account_id=9&account_id=2&account_id=9&include_descendants=false&reporting_currency_id=7&fx_method=constant_as_of'));
    // Learned-spending fields default to off, so a core-only URL is unchanged in meaning.
    expect(parsed).toEqual({
      valid: true,
      filters: { ...defaultForecastFilters, horizonDays: 30, accountIDs: [2, 9], includeDescendants: false, reportingCurrencyID: 7 }
    });
    if (!parsed.valid) return;
    expect(writeForecastFilters(parsed.filters).toString()).toBe('horizon_days=30&account_id=2&account_id=9&include_descendants=false&reporting_currency_id=7&fx_method=constant_as_of');
  });

  it.each(['horizon_days=0', 'horizon_days=1.5', 'include_descendants=1', 'account_id=-1', 'reporting_currency_id=7', 'fx_method=constant_as_of', 'other=1'])(
    'rejects malformed URL state: %s',
    (query) => expect(parseForecastFilters(new URLSearchParams(query)).valid).toBe(false)
  );
});

describe('forecast presentation model', () => {
  it('uses exact BigInt geometry for large signed values and a shared two-line scale', () => {
    const points = forecastChartPoints(series(['-9007199254740993', '0'], ['9007199254740993', '0']));
    expect(points[0]).toMatchObject({ recordedY: 1, projectedY: 0 });
    expect(points[1]).toMatchObject({ recordedY: 0.5, projectedY: 0.5 });
  });

  it('keeps zero points and does not mutate its source series', () => {
    const input = series(['0', '0']);
    const before = structuredClone(input);
    expect(forecastChartPoints(input)).toHaveLength(2);
    expect(forecastHasMovements([input])).toBe(false);
    expect(input).toEqual(before);
  });
});

describe('learned spending filters', () => {
  it('rejects a model parameter with the model off', () => {
    for (const search of ['history_complete_from=2024-01-01', 'expense_category_id=3', 'expense_pattern=3:weekly']) {
      const result = parseForecastFilters(new URLSearchParams(search));
      expect(result.valid).toBe(false);
    }
  });

  it('accepts a complete opt-in recipe and round-trips it', () => {
    const params = new URLSearchParams(
      'spending_model=adaptive_v1&history_complete_from=2024-01-01&expense_category_id=7&expense_category_id=3&expense_pattern=3:annual_seasonal'
    );
    const result = parseForecastFilters(params);
    expect(result.valid).toBe(true);
    if (!result.valid) return;
    expect(result.filters.spendingModel).toBe('adaptive_v1');
    expect(result.filters.historyCompleteFrom).toBe('2024-01-01');
    expect(result.filters.expenseCategoryIDs).toEqual([3, 7]);
    expect(result.filters.expensePatterns).toEqual({ 3: 'annual_seasonal' });
    expect(writeForecastFilters(result.filters).getAll('expense_pattern')).toEqual(['3:annual_seasonal']);
    expect(writeForecastFilters(result.filters).getAll('expense_category_id')).toEqual(['3', '7']);
  });

  it('rejects invalid patterns, duplicates and out-of-scope overrides', () => {
    const invalid = [
      'spending_model=adaptive_v1&history_complete_from=2024-01-01&expense_pattern=3:quarterly',
      'spending_model=adaptive_v1&history_complete_from=2024-01-01&expense_pattern=3:weekly&expense_pattern=3:monthly',
      'spending_model=adaptive_v1&history_complete_from=2024-01-01&expense_category_id=3&expense_pattern=9:weekly',
      'spending_model=adaptive_v1&history_complete_from=not-a-date',
      'spending_model=adaptive_v1',
      'spending_model=neural_v9&history_complete_from=2024-01-01'
    ];
    for (const search of invalid) {
      expect(parseForecastFilters(new URLSearchParams(search)).valid, search).toBe(false);
    }
  });

  it('writes no model parameters when estimates are off', () => {
    const params = writeForecastFilters({ ...defaultForecastFilters, historyCompleteFrom: '2024-01-01', expenseCategoryIDs: [3] });
    expect(params.has('spending_model')).toBe(false);
    expect(params.has('history_complete_from')).toBe(false);
    expect(params.has('expense_category_id')).toBe(false);
  });

  it('plots the estimated curve alongside the core curves', () => {
    const series = {
      commodity_id: 1,
      commodity_code: 'EUR',
      points: [
        { date: '2026-01-01', recorded_balance: { quantity_value: '10000', quantity_scale: 2 }, projected_balance: { quantity_value: '10000', quantity_scale: 2 } },
        { date: '2026-01-02', recorded_balance: { quantity_value: '10000', quantity_scale: 2 }, projected_balance: { quantity_value: '9000', quantity_scale: 2 } }
      ]
    } as unknown as ForecastSeries;
    const estimated = {
      commodity_id: 1,
      points: [
        { date: '2026-01-01', estimated_delta: { quantity_value: '0', quantity_scale: 2 }, projected_balance: { quantity_value: '9500', quantity_scale: 2 } },
        { date: '2026-01-02', estimated_delta: { quantity_value: '-500', quantity_scale: 2 }, projected_balance: { quantity_value: '8000', quantity_scale: 2 } }
      ]
    } as unknown as ForecastLearnedSeries;

    const withEstimates = forecastChartPoints(series, estimated);
    expect(withEstimates.every((point) => point.estimatedY !== undefined)).toBe(true);
    // The plot is anchored at zero, so 80.00 of a 100.00 span sits at 0.2 and
    // the estimated curve always renders below the projected one.
    expect(withEstimates[1].estimatedY).toBe(0.2);
    expect(withEstimates[1].estimatedY!).toBeGreaterThan(withEstimates[1].projectedY);

    // Without the overlay no estimated point is produced, and the core curves
    // keep exactly the scaling they had before the option existed.
    const core = forecastChartPoints(series);
    expect(core.every((point) => point.estimatedY === undefined)).toBe(true);
    expect(core.map((point) => point.projectedY)).toEqual(withEstimates.map((point) => point.projectedY));
  });

  it('uses the separately converted learned curve for the converted core row', () => {
    const native = { commodity_id: 1, points: [] } as unknown as ForecastLearnedSeries;
    const converted = { commodity_id: 1, points: [{ date: '2026-01-01' }] } as unknown as ForecastLearnedSeries;
    const learned = {
      status: 'ready',
      totals: [native],
      series: [],
      converted
    } as unknown as ForecastLearnedSpending;

    expect(forecastLearnedSeriesFor(learned, undefined, 1)).toBe(native);
    expect(forecastLearnedSeriesFor(learned, undefined, 1, true)).toBe(converted);
  });
});
