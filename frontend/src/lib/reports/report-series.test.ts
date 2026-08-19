import { describe, expect, it } from 'vitest';
import { hasNegativeColumn, seriesColumns, type SeriesPoint } from './report-series';

const point = (key: string, quantityValue: string, quantityScale = 2): SeriesPoint => ({
  key,
  label: key,
  quantityValue,
  quantityScale
});

describe('seriesColumns', () => {
  it('scales against the largest magnitude and keeps the sign', () => {
    const columns = seriesColumns([point('a', '10000'), point('b', '-5000'), point('c', '2500')]);
    expect(columns[0].ratio).toBeCloseTo(1);
    expect(columns[1].ratio).toBeCloseTo(-0.5);
    expect(columns[2].ratio).toBeCloseTo(0.25);
    expect(columns[1].negative).toBe(true);
  });

  it('mirrors equal magnitudes of opposite sign', () => {
    const columns = seriesColumns([point('saved', '50000'), point('burned', '-50000')]);
    expect(columns[0].ratio).toBeCloseTo(1);
    expect(columns[1].ratio).toBeCloseTo(-1);
  });

  it('compares unlike scales by value, not by coefficient width', () => {
    // 1.00 at scale 2 against 0.5000 at scale 4: the first is the taller.
    const columns = seriesColumns([point('a', '100', 2), point('b', '5000', 4)]);
    expect(columns[0].ratio).toBeCloseTo(1);
    expect(columns[1].ratio).toBeCloseTo(0.5);
  });

  it('keeps a zero bucket as a zero column rather than dropping it', () => {
    const columns = seriesColumns([point('a', '10000'), point('b', '0')]);
    expect(columns).toHaveLength(2);
    expect(columns[1].ratio).toBe(0);
    expect(columns[1].negative).toBe(false);
  });

  it('gives every column a zero ratio when nothing moved', () => {
    expect(seriesColumns([point('a', '0'), point('b', '0')]).map((c) => c.ratio)).toEqual([0, 0]);
  });

  it('survives a coefficient no float could hold', () => {
    const columns = seriesColumns([
      point('huge', '12345678901234567890123456789012345678', 2),
      point('small', '100', 2)
    ]);
    expect(columns[0].ratio).toBeCloseTo(1);
    expect(columns[1].ratio).toBeLessThan(0.000001);
  });

  it('sizes no column from a value it cannot read', () => {
    const columns = seriesColumns([point('bad', 'not-a-number'), point('good', '10000')]);
    expect(columns[0].ratio).toBe(0);
    expect(columns[1].ratio).toBeCloseTo(1);
  });
});

describe('hasNegativeColumn', () => {
  it('reports whether the chart needs a lower half', () => {
    expect(hasNegativeColumn(seriesColumns([point('a', '10000')]))).toBe(false);
    expect(hasNegativeColumn(seriesColumns([point('a', '-10000')]))).toBe(true);
  });
});
