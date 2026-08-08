import { describe, it, expect } from 'vitest';
import { chartBars, linePath, normalizePoints, type ChartPoint } from './report-chart';

const point = (value: string, scale = 2, label = ''): ChartPoint => ({ value, scale, label });

describe('normalizePoints', () => {
  it('returns nothing for fewer than two points', () => {
    // One point has no range; drawing it at any height implies a trend the
    // data does not contain.
    expect(normalizePoints([])).toEqual([]);
    expect(normalizePoints([point('10000')])).toEqual([]);
  });

  it('measures against a zero baseline, not against the series minimum', () => {
    // 100 and 200 against a baseline of 0 are half and full height. Using the
    // series minimum instead would draw 100 at zero and exaggerate a mild rise
    // into a climb from nothing.
    expect(normalizePoints([point('10000'), point('20000')])).toEqual([0.5, 1]);
  });

  it('extends the range below zero when values go negative', () => {
    // -100 .. 100 spans 200, so -100 sits at the floor, 0 at the middle.
    expect(normalizePoints([point('-10000'), point('0'), point('10000')])).toEqual([0, 0.5, 1]);
  });

  it('draws an all-zero series flat at the baseline', () => {
    expect(normalizePoints([point('0'), point('0')])).toEqual([0, 0]);
  });

  it('aligns points carrying different scales before comparing', () => {
    // 1.00 (scale 2) and 1.0000 (scale 4) are the same value, so both sit at
    // full height against the zero baseline.
    expect(normalizePoints([point('100', 2), point('10000', 4)])).toEqual([1, 1]);
  });

  it('normalises coefficients beyond Number.MAX_SAFE_INTEGER without corruption', () => {
    // These two differ only in their last digit, well past the point where a
    // float would collapse them together.
    const fractions = normalizePoints([
      point('9007199254740993000000'),
      point('18014398509481986000000')
    ]);

    expect(fractions).toEqual([0.5, 1]);
  });
});

describe('linePath', () => {
  it('spreads points evenly across the width and inverts the y axis', () => {
    // The larger value must sit higher, i.e. at a smaller y.
    expect(linePath([point('10000'), point('20000')], 100, 40)).toBe('0.00,20.00 100.00,0.00');
  });

  it('returns an empty path when there is nothing to draw', () => {
    expect(linePath([point('10000')], 100, 40)).toBe('');
  });
});

describe('chartBars', () => {
  it('sizes bars against the widest magnitude', () => {
    const bars = chartBars([point('10000', 2, 'Rent'), point('5000', 2, 'Food')]);

    expect(bars.map((bar) => bar.fraction)).toEqual([1, 0.5]);
    expect(bars.map((bar) => bar.label)).toEqual(['Rent', 'Food']);
  });

  it('gives a negative net a visible bar and flags it rather than drawing zero', () => {
    // A category that nets negative after refunds is real information; a
    // zero-length bar would hide it.
    const bars = chartBars([point('10000', 2, 'Rent'), point('-5000', 2, 'Electronics')]);

    expect(bars[1].fraction).toBe(0.5);
    expect(bars[1].negative).toBe(true);
    expect(bars[0].negative).toBe(false);
  });

  it('draws every bar at zero when all values are zero', () => {
    expect(chartBars([point('0', 2, 'A'), point('0', 2, 'B')]).map((bar) => bar.fraction)).toEqual([0, 0]);
  });

  it('returns nothing for an empty set', () => {
    expect(chartBars([])).toEqual([]);
  });

  it('aligns mixed scales before sizing', () => {
    // 1.00 vs 0.5000 — the second is half the first despite carrying a larger
    // raw coefficient.
    const bars = chartBars([point('100', 2, 'A'), point('5000', 4, 'B')]);

    expect(bars.map((bar) => bar.fraction)).toEqual([1, 0.5]);
  });
});
