/**
 * Geometry for the reports' summary charts.
 *
 * These charts summarize the table beside them and never carry information the
 * table lacks — the table stays the accessible source of truth, and the SVG is
 * marked `aria-hidden`. A chart is only offered when a single commodity is
 * selected, because plotting unlike commodities on one axis draws exactly the
 * comparison the reports refuse to make numerically.
 *
 * ## Precision
 *
 * Values arrive as exact integer coefficient strings that may exceed
 * `Number.MAX_SAFE_INTEGER`. Converting one to a `Number` to find its pixel
 * position would corrupt large books. So every comparison and the normalisation
 * itself run in `BigInt`; a `Number` appears only at the very last step, as a
 * 0–10000 integer ratio that is by construction small. A pixel is a pixel — the
 * rounding there is presentation, never money.
 */

/** Scale used for the integer ratio the chart normalises through. */
const RATIO_SCALE = 10_000n;

export interface ChartPoint {
  /** Coefficient string, exact. */
  value: string;
  /** Scale the coefficient carries. */
  scale: number;
  /** Label for the point, used in the table, not drawn. */
  label: string;
}

/** Restate a coefficient at a deeper scale so points at mixed scales compare. */
function aligned(point: ChartPoint, toScale: number): bigint {
  return BigInt(point.value) * 10n ** BigInt(toScale - point.scale);
}

function maxScale(points: ChartPoint[]): number {
  return points.reduce((widest, point) => Math.max(widest, point.scale), 0);
}

/**
 * Normalise points to 0–1 fractions of the value range.
 *
 * Returns an empty array for fewer than two points: a single point has no
 * range, and drawing it at an arbitrary height would imply a trend that is not
 * in the data.
 *
 * The range always includes zero, so a chart of values that never dip negative
 * is read against a real baseline rather than against its own minimum — which
 * would exaggerate a flat series into a dramatic slope.
 */
export function normalizePoints(points: ChartPoint[]): number[] {
  if (points.length < 2) return [];

  const scale = maxScale(points);
  const values = points.map((point) => aligned(point, scale));

  let min = 0n;
  let max = 0n;
  for (const value of values) {
    if (value < min) min = value;
    if (value > max) max = value;
  }

  const span = max - min;
  if (span === 0n) {
    // Every value is zero. A flat line at the baseline is the honest picture.
    return values.map(() => 0);
  }

  return values.map((value) => {
    const ratio = ((value - min) * RATIO_SCALE) / span;
    return Number(ratio) / Number(RATIO_SCALE);
  });
}

/**
 * An SVG polyline path for a line chart, in a `width` x `height` viewBox.
 *
 * Y is inverted because SVG's origin is top-left while a larger value should
 * sit higher.
 */
export function linePath(points: ChartPoint[], width: number, height: number): string {
  const fractions = normalizePoints(points);
  if (fractions.length === 0) return '';

  const step = fractions.length === 1 ? 0 : width / (fractions.length - 1);
  return fractions
    .map((fraction, index) => {
      const x = (index * step).toFixed(2);
      const y = (height - fraction * height).toFixed(2);
      return `${x},${y}`;
    })
    .join(' ');
}

export interface ChartBar {
  label: string;
  /** 0–1 fraction of the widest bar. */
  fraction: number;
  /** True when the underlying value is negative, e.g. a category net of refunds. */
  negative: boolean;
}

/**
 * Bars sized against the largest magnitude in the set.
 *
 * Magnitude, not signed value: a category that nets negative after refunds
 * still deserves a visible bar, distinguished by `negative` rather than by
 * being drawn at zero length.
 */
export function chartBars(points: ChartPoint[]): ChartBar[] {
  if (points.length === 0) return [];

  const scale = maxScale(points);
  const magnitudes = points.map((point) => {
    const value = aligned(point, scale);
    return value < 0n ? -value : value;
  });

  const widest = magnitudes.reduce((best, magnitude) => (magnitude > best ? magnitude : best), 0n);
  if (widest === 0n) {
    return points.map((point) => ({ label: point.label, fraction: 0, negative: false }));
  }

  return points.map((point, index) => ({
    label: point.label,
    fraction: Number((magnitudes[index] * RATIO_SCALE) / widest) / Number(RATIO_SCALE),
    negative: point.value.startsWith('-')
  }));
}
