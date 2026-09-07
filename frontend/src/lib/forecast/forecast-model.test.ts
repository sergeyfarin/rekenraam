import { describe, expect, it } from 'vitest';
import { forecastChartPoints, forecastHasMovements, parseForecastFilters, writeForecastFilters, type ForecastSeries } from './forecast-model';

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
    expect(parsed).toEqual({ valid: true, filters: { horizonDays: 30, accountIDs: [2, 9], includeDescendants: false, reportingCurrencyID: 7 } });
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
