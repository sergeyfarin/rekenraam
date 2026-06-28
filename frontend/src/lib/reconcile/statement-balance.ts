/**
 * Parse a user-entered statement balance into a lossless debit-positive integer
 * coefficient + scale, matching the ledger's value/scale money representation.
 *
 * The scale is the number of fractional digits the user actually typed; the backend
 * stores it as-is (it does not need to match the currency's standard scale). Returns
 * `null` for anything that is not a valid signed decimal so the form can reject it
 * rather than send a malformed amount.
 */
export function parseStatementBalance(input: string): { value: string; scale: number } | null {
  const trimmed = input.trim().replace(/,/g, '');
  if (!trimmed) return null;

  const isNeg = trimmed.startsWith('-');
  const abs = isNeg ? trimmed.slice(1) : trimmed;
  const dotIdx = abs.indexOf('.');

  let intStr: string;
  let fracStr: string;
  let scale: number;

  if (dotIdx === -1) {
    intStr = abs || '0';
    fracStr = '';
    scale = 0;
  } else {
    // Reject a second dot or any non-digit by falling through to the regex check.
    intStr = abs.slice(0, dotIdx) || '0';
    fracStr = abs.slice(dotIdx + 1);
    scale = fracStr.length;
  }

  const rawCoefficient = intStr + fracStr;
  if (!/^\d+$/.test(rawCoefficient)) return null;

  // Canonicalise to the minimal integer coefficient (strip leading zeros, keep one
  // digit). The scale is unchanged, so "0.05" → value "5" scale 2, and "0.00" → "0".
  const coefficient = rawCoefficient.replace(/^0+/, '') || '0';

  // A zero magnitude must never carry a sign, so "-0.00" → "0".
  const value = isNeg && coefficient !== '0' ? `-${coefficient}` : coefficient;
  return { value, scale };
}
