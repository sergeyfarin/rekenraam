import { amountPrivacy } from './privacy.svelte.ts';

export const hiddenAmount = '••••';

/** Raw source evidence still uses the same presentation privacy boundary. */
export function maskFinancialText(value: string): string {
  return amountPrivacy.hidden && value ? hiddenAmount : value;
}

import { divideAmountsForDisplay, roundAmountForDisplay, type ScaledAmount } from './amount';

/**
 * The frontend's single money-*display* layer.
 *
 * `#lib/money/amount.ts` owns parsing, exact arithmetic and editable decimals:
 * `formatEditableAmount` round-trips through the locale-aware
 * `parseDecimalAmount`, while `formatLedgerAmount` keeps machine-readable values
 * locale-independent. This module is the other half —
 * locale-aware presentation of **read-only** money: report tables, registers,
 * balances, CSV headers' worth of figures the user reads but never edits.
 *
 * Nothing here is authoritative. Canonical balances and report figures are
 * computed in Go; this module only decides how an already-exact
 * `{ value, scale }` pair looks to a reader.
 *
 * ## Why `Intl.NumberFormat` and not Dinero.js (G-08)
 *
 * `dinero.js` 2.0.2 was a declared dependency with zero imports, held in
 * reserve as "the locale-aware display layer". Settled 2026-08-08 against it,
 * and the dependency was dropped:
 *
 * - Dinero's own formatting is a thin wrapper that hands off to
 *   `Intl.NumberFormat` anyway, so it buys no formatting capability we do not
 *   already have below.
 * - Using it would mean converting every `{ value, scale }` pair into a Dinero
 *   object first. Dinero's default calculator is JS numbers, which breaks past
 *   `Number.MAX_SAFE_INTEGER`; avoiding that means wiring its BigInt
 *   calculator, i.e. taking on a dependency to reach the arithmetic we already
 *   do directly and test directly.
 * - The ledger's scale is per-value, not per-currency. Dinero models money as
 *   an amount in a currency with a fixed exponent, which is the wrong shape for
 *   a book that stores a 24-scale crypto quantity next to a 2-scale euro.
 *
 * Locale-aware editable input is implemented separately in `amount.ts` (#100).
 */

/**
 * Format an integer-coefficient + scale amount for display without ever
 * converting to a JS number (which would lose precision beyond
 * `Number.MAX_SAFE_INTEGER`).
 *
 * Safe path: string arithmetic to split integer/fractional parts, then
 * `Intl.NumberFormat` over a `BigInt` to obtain locale-appropriate group
 * separators. The fractional digits are emitted verbatim, so a value's stored
 * scale survives display: a 4-scale FX rate does not get rounded to 2.
 *
 * @param value  The integer coefficient as a decimal string (e.g. "12345", "-12345").
 * @param scale  Number of decimal places (e.g. 2 → coefficient 12345 = 123.45).
 * @param locale BCP-47 locale tag (e.g. "en-US").
 */
export function formatQuantity(value: string, scale: number, locale: string): string {
  if (!value || value === '') return '';
  if (amountPrivacy.hidden) return hiddenAmount;

  const isNegative = value.startsWith('-');
  const absStr = isNegative ? value.slice(1) : value;

  if (scale === 0) {
    // Integer amount — BigInt handles arbitrarily large values losslessly.
    const intPart = BigInt(absStr);
    const formatted = new Intl.NumberFormat(locale).format(intPart);
    return isNegative ? `-${formatted}` : formatted;
  }

  // Split coefficient into integer and fractional parts by string arithmetic.
  let intStr: string;
  let fracStr: string;

  if (absStr.length <= scale) {
    // Coefficient is smaller than one unit (e.g. "45", scale 2 → "0.45").
    intStr = '0';
    fracStr = absStr.padStart(scale, '0');
  } else {
    intStr = absStr.slice(0, absStr.length - scale);
    fracStr = absStr.slice(absStr.length - scale);
  }

  // Use Intl with a BigInt to get locale-aware thousands grouping on the integer part.
  const intPartGrouped = new Intl.NumberFormat(locale).format(BigInt(intStr));

  // Get the locale decimal separator from Intl parts.
  const parts = new Intl.NumberFormat(locale, { minimumFractionDigits: 1 }).formatToParts(0);
  const decimalSep = parts.find((p) => p.type === 'decimal')?.value ?? '.';

  const result = `${intPartGrouped}${decimalSep}${fracStr}`;
  return isNegative ? `-${result}` : result;
}

/**
 * Joins a commodity's label to an amount with a separator only where one is
 * needed (T-62).
 *
 * A currency symbol that is punctuation reads correctly against its amount:
 * `$42.00`, `€42.00`. A label that ends in a letter or digit does not — an
 * instrument ticker or a currency with no symbol runs the two together and
 * produces `AAPL2.000` or `USD42.00`, which is a different number to a quick
 * reader. The rule is stated once here rather than judged at each call site,
 * which is how the three existing call sites came to disagree with the reports
 * views about the same question.
 */
export function joinCommodityAmount(label: string, amount: string): string {
	const trimmed = label.trim();
	if (trimmed === '') return amount;
	return endsAlphanumeric(trimmed) ? `${trimmed} ${amount}` : `${trimmed}${amount}`;
}

function endsAlphanumeric(label: string): boolean {
	const last = label.at(-1);
	if (last === undefined) return false;
	// Unicode-aware: a Cyrillic or Greek currency abbreviation is a word too.
	return /\p{L}|\p{N}/u.test(last);
}

/** Currency summary display. Call only after exact aggregation, with the
 * currency's standard scale (not its maximum input/allocation scale). */
export function formatMoney(value: string, scale: number, standardScale: number, locale: string): string {
	return formatQuantity(roundAmountForDisplay(value, scale, standardScale), standardScale, locale);
}

/** Home's summary display (#476): an amount of 100 or more in its currency
 * shows no minor units (€56,750), rounded half up — ties away from zero, as
 * every display rounding here — for display only; a smaller amount keeps the
 * currency's standard scale (€96.40). Only Home's briefing and widgets use it;
 * registers, reports, menus, editors and exports stay exact. */
export function formatHomeMoney(value: string, scale: number, standardScale: number, locale: string): string {
	// Decide on the amount as it would otherwise show: 99.995 shows as 100.00,
	// so it is 100 or more.
	const shown = BigInt(roundAmountForDisplay(value, scale, standardScale));
	const magnitude = shown < 0n ? -shown : shown;
	if (magnitude >= 100n * 10n ** BigInt(standardScale)) return formatQuantity(roundAmountForDisplay(value, scale, 0), 0, locale);
	return formatQuantity(shown.toString(), standardScale, locale);
}

/** Exact money display for disclosures that must never round: drops only the
 * trailing zeros below the coefficient's own scale, keeping at least the
 * currency's standard scale. A replay result stored at scale 6 shows `50.00`,
 * while a genuine `50.004` still shows every digit (T-114). */
export function formatExactMoney(value: string, scale: number, standardScale: number, locale: string): string {
	if (/^-?0+$/.test(value)) return formatQuantity('0', standardScale, locale);
	let coefficient = value;
	let displayScale = scale;
	while (displayScale > standardScale && coefficient.endsWith('0')) {
		coefficient = coefficient.slice(0, -1);
		displayScale--;
	}
	if (displayScale < standardScale) {
		coefficient += '0'.repeat(standardScale - displayScale);
		displayScale = standardScale;
	}
	return formatQuantity(coefficient, displayScale, locale);
}

/** Implied-price display, e.g. an FX rate stored as `price` per `quantity`:
 * divides exactly, rounds half-up (ties away from zero) at `maxDecimals`, and
 * drops trailing zeros. A zero quantity has no rate and shows an em dash. */
export function formatRatio(
	price: ScaledAmount,
	quantity: ScaledAmount,
	maxDecimals: number,
	locale: string
): string {
	let coefficient = divideAmountsForDisplay(price, quantity, maxDecimals);
	if (coefficient === null) return '\u2014';
	let scale = maxDecimals;
	while (scale > 0 && coefficient.endsWith('0')) {
		coefficient = coefficient.slice(0, -1);
		scale--;
	}
	if (/^-?0*$/.test(coefficient)) return formatQuantity('0', 0, locale);
	return formatQuantity(coefficient, scale, locale);
}
