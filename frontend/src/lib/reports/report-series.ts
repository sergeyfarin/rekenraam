/**
 * Column geometry for the time-series chart summaries.
 *
 * Net worth and cashflow both summarize a bucket table as one signed series per
 * commodity, so the geometry lives here rather than twice. Like
 * `spendingBars`, this is presentation only: every displayed amount still comes
 * from the exact coefficient strings.
 */

export type SeriesPoint = {
  key: string;
  label: string;
  quantityValue: string;
  quantityScale: number;
};

export type SeriesColumn = SeriesPoint & {
  /** Signed fraction of the largest magnitude in the set, -1..1. */
  ratio: number;
  negative: boolean;
};

/** Fixed-point denominator for the final ratio division. */
const RATIO_PRECISION = 1_000_000_000_000_000n;

function scaleOf(point: SeriesPoint): number {
  return Number.isInteger(point.quantityScale) && point.quantityScale > 0 ? point.quantityScale : 0;
}

/**
 * A point's signed value as an exact BigInt at a common scale.
 *
 * BigInt rather than Number because a 38-digit coefficient or a scale-24 crypto
 * quantity does not survive a float, and the tallest column would be picked
 * against the wrong track. A coefficient the backend could not have produced
 * sizes no column rather than throwing.
 */
function valueOf(point: SeriesPoint, scale: number): bigint {
  let coefficient: bigint;
  try {
    coefficient = BigInt(point.quantityValue);
  } catch {
    return 0n;
  }
  return coefficient * 10n ** BigInt(scale - scaleOf(point));
}

function ratioOf(value: bigint, tallest: bigint): number {
  const scaled = (value * RATIO_PRECISION) / tallest;
  return Number(scaled) / Number(RATIO_PRECISION);
}

/**
 * Scales a series against its own largest magnitude, keeping the sign.
 *
 * Positive and negative share one track so the columns stay comparable: a month
 * that burned 500 must read as the mirror of one that saved 500, not as its
 * own full-height bar. Zero points keep a zero ratio rather than being dropped,
 * so the chart never disagrees with the table about which buckets exist.
 */
export function seriesColumns(points: SeriesPoint[]): SeriesColumn[] {
  const scale = points.reduce((max, point) => Math.max(max, scaleOf(point)), 0);
  const values = points.map((point) => valueOf(point, scale));
  const tallest = values.reduce((max, value) => {
    const magnitude = value < 0n ? -value : value;
    return magnitude > max ? magnitude : max;
  }, 0n);

  return points.map((point, index) => ({
    ...point,
    ratio: tallest === 0n ? 0 : ratioOf(values[index], tallest),
    negative: values[index] < 0n
  }));
}

/** True when any column drops below the baseline, so the chart needs both halves. */
export function hasNegativeColumn(columns: SeriesColumn[]): boolean {
  return columns.some((column) => column.negative);
}
