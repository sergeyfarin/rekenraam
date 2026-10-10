/** Largest accepted side of a split ratio, matching the API bound. */
export const SPLIT_RATIO_MAX = 1_000_000_000;

type ParsedRatio = { ok: true; numerator: number; denominator: number } | { ok: false };

// Digits only, no signs, separators or decimals, within the API bound.
function parseRatioSide(raw: string): number | null {
  const text = raw.trim();
  if (!/^[0-9]{1,10}$/.test(text)) return null;
  const value = Number(text);
  return value >= 1 && value <= SPLIT_RATIO_MAX ? value : null;
}

/**
 * Parse the two whole-number sides of a split ratio typed by the user.
 * Digits only, no signs, separators or decimals, both within the API bound,
 * and not the same corporate action as 1-for-1 once reduced.
 */
export function parseSplitRatio(newUnits: string, oldUnits: string): ParsedRatio {
  const ratio = parseExchangeRatio(newUnits, oldUnits);
  return ratio.ok && ratio.numerator === ratio.denominator ? { ok: false } : ratio;
}

/**
 * Parse a share exchange ratio (#178): new units received for old units
 * surrendered. Equal sides are a one-for-one class conversion.
 */
export function parseExchangeRatio(newUnits: string, oldUnits: string): ParsedRatio {
  const numerator = parseRatioSide(newUnits);
  const denominator = parseRatioSide(oldUnits);
  if (numerator === null || denominator === null) return { ok: false };
  return { ok: true, numerator, denominator };
}

type ParsedFraction = { ok: true; value: string; scale: number } | { ok: false };

/**
 * Parse a spin-off basis allocation typed as a percentage (#180), such as
 * "14.1" or "14,1", into the exact fraction the API stores: 14.1 % is value
 * "141" at scale 3. One decimal mark, '.' or ',', and never a grouping
 * separator, so a decimal comma can never scale the figure by 100 or 1000.
 * The fraction must lie strictly between 0 and 1 with at most 12 decimals.
 */
export function parseBasisPercent(raw: string): ParsedFraction {
  const match = /^([0-9]{1,3})(?:[.,]([0-9]{1,10}))?$/.exec(raw.trim());
  if (!match) return { ok: false };
  const fractionDigits = (match[2] ?? '').replace(/0+$/, '');
  const coefficient = (match[1] + fractionDigits).replace(/^0+/, '');
  if (!coefficient) return { ok: false };
  const scale = fractionDigits.length + 2;
  // Below 100 %: the coefficient must have fewer digits than the scale.
  if (coefficient.length > scale || scale > 12) return { ok: false };
  // Trailing zeros of the integer part move into the scale (10 % is 1/10).
  let value = coefficient;
  let lowest = scale;
  while (lowest > 1 && value.endsWith('0')) {
    value = value.slice(0, -1);
    lowest--;
  }
  return { ok: true, value, scale: lowest };
}

/**
 * The exact percentage a stored basis fraction stands for, as a coefficient
 * and scale for display: value/10^scale × 100.
 */
export function basisFractionPercent(value: string, scale: number): { value: string; scale: number } {
  return scale >= 2 ? { value, scale: scale - 2 } : { value: value + '0'.repeat(2 - scale), scale: 0 };
}
