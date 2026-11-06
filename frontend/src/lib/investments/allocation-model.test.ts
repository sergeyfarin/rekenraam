import { describe, expect, it } from 'vitest';
import type { AllocationPoint } from '#lib/api/allocation.ts';
import { allocationAreas, allocationIDs, calendarFraction, parseTargetUnits, savedCalendarDay } from './allocation-model.ts';

describe('exact target entry', () => {
  it('preserves four decimal percentage places in each maintained locale', () => {
    expect(parseTargetUnits('33.3333', 'en')).toBe(333333);
    expect(parseTargetUnits('33,3334', 'nl')).toBe(333334);
    expect(parseTargetUnits('33,3333', 'ru')).toBe(333333);
    expect(parseTargetUnits('100', 'nl')).toBe(1000000);
  });
  it('refuses negative, excessive, ambiguous and over-precise targets', () => {
    for (const input of ['-1', '100.0001', '33.33333', '1,50', '', 'NaN']) expect(parseTargetUnits(input, 'en')).toBeNull();
    expect(parseTargetUnits('1.50', 'nl')).toBeNull();
  });
});

const point = (date: string, weights: [number, string][], chart_reason = ''): AllocationPoint => ({
  date, chart_reason, reason: chart_reason, total: null, unknown: [], negative_cash: null,
  rows: weights.map(([commodity_id, weight]) => ({ commodity_id, side: 'long', weight, amount: null, target_units: null, drift: null }))
});
describe('allocation chart geometry', () => {
  it('places cash last and stacks component shares cumulatively', () => {
    const points = [point('2026-01-01', [[0, '0.100000'], [2, '0.600000'], [3, '0.300000']])];
    const ids = allocationIDs(points, []);
    expect(ids).toEqual([2, 3, 0]);
    const areas = allocationAreas(points, ids);
    expect(areas[0].segments[0][0]).toEqual({ x: 0.5, lower: 0, upper: 0.6 });
    expect(areas[2].segments[0][0].upper).toBeCloseTo(1);
  });
  it('breaks bands at missing or signed samples and preserves recovered weights', () => {
    const points = [point('2026-01-01', [[2, '0.900000']]), point('2026-01-02', [], 'valuation_unavailable'), point('2026-01-03', [[2, '0.950000']]), point('2026-01-04', [], 'signed_allocation')];
    const segments = allocationAreas(points, [2])[0].segments;
    expect(segments).toHaveLength(2);
    expect(segments.map(s => s.length)).toEqual([1, 1]);
    expect(segments[1][0].upper).toBe(0.95);
  });
  it('uses calendar distance across leap days, with a centred one-day sample', () => {
    expect(calendarFraction('2024-02-29', '2024-02-28', '2024-03-01')).toBe(0.5);
    expect(calendarFraction('2026-01-01', '2026-01-01', '2026-01-01')).toBe(0.5);
  });
  it('places saved-change timestamps in the explicit owner zone', () => {
    expect(savedCalendarDay('2026-11-05T23:30:00Z', 'Europe/Amsterdam')).toBe('2026-11-06');
    expect(savedCalendarDay('2026-11-05T23:30:00Z', 'America/New_York')).toBe('2026-11-05');
  });
});
