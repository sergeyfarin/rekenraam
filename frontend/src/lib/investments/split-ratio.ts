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
