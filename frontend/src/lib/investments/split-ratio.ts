/** Largest accepted side of a split ratio, matching the API bound. */
export const SPLIT_RATIO_MAX = 1_000_000_000;

/**
 * Parse the two whole-number sides of a split ratio typed by the user.
 * Digits only, no signs, separators or decimals, both within the API bound,
 * and not the same corporate action as 1-for-1 once reduced.
 */
export function parseSplitRatio(newUnits: string, oldUnits: string):
  { ok: true; numerator: number; denominator: number } | { ok: false } {
  const parse = (raw: string): number | null => {
    const text = raw.trim();
    if (!/^[0-9]{1,10}$/.test(text)) return null;
    const value = Number(text);
    return value >= 1 && value <= SPLIT_RATIO_MAX ? value : null;
  };
  const numerator = parse(newUnits);
  const denominator = parse(oldUnits);
  if (numerator === null || denominator === null || numerator === denominator) return { ok: false };
  return { ok: true, numerator, denominator };
}
