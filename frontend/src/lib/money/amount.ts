import { getLocale } from '#lib/paraglide/runtime.js';

/**
 * The frontend's single money-parsing and money-formatting layer.
 *
 * Everything here is pure string/BigInt arithmetic over the ledger's
 * `{ value: integer coefficient, scale: fractional digits }` representation.
 * No `Number` ever touches a coefficient, so values well past
 * `Number.MAX_SAFE_INTEGER` round-trip losslessly.
 *
 * Scope: input parsing, display formatting, and the editor's client-side
 * balance hint. Canonical balances and report figures are computed in Go —
 * see the `ledger-invariants` skill. Nothing here is authoritative.
 */

/** The ledger's exact-money shape: a signed integer coefficient + a scale. */
export interface ScaledAmount {
	/** Canonical signed base-10 integer coefficient, e.g. "-4210". */
	value: string;
	/** Number of fractional digits the coefficient carries, e.g. 2. */
	scale: number;
}

/**
 * Backend ceiling (`exact.MaxCoefficientDigits`). Rejecting here turns what
 * would be an opaque 400 into an inline form error.
 */
export const MAX_COEFFICIENT_DIGITS = 38;

/**
 * Absolute scale ceiling (`exact.MaxCryptoScale`). A commodity's own
 * `max_quantity_scale` may be lower — callers that know it pass `maxScale`.
 */
export const MAX_SUPPORTED_SCALE = 24;

const separatorsByLocale = new Map<string, { decimal: string; group: string }>();

function amountSeparators(locale: string) {
	let separators = separatorsByLocale.get(locale);
	if (!separators) {
		// Only a fixed sample passes through Number; amount digits never do.
		const parts = new Intl.NumberFormat(locale).formatToParts(12345.6);
		separators = {
			decimal: parts.find(part => part.type === 'decimal')!.value,
			group: parts.find(part => part.type === 'group')!.value
		};
		separatorsByLocale.set(locale, separators);
	}
	return separators;
}

/**
 * Parse a user-entered decimal string into a canonical coefficient + scale.
 *
 * The scale is the number of fractional digits the user actually typed; the
 * backend stores it as-is and does not require it to match the commodity's
 * standard scale. Returns `null` for anything that is not a valid signed
 * decimal so callers can reject the input instead of sending a malformed
 * amount. The active locale decides decimal/group separators; no fallback to
 * another locale or separator guessing is permitted. All six shipping locales
 * group in threes. Space-grouping locales also accept keyboard space, NBSP and
 * narrow NBSP, but a single amount must use one consistent grouping character.
 */
export function parseDecimalAmount(
	input: string,
	options: { maxScale?: number; locale?: string } = {}
): ScaledAmount | null {
	const trimmed = input.trim();
	if (!trimmed) return null;

	const isNeg = trimmed.startsWith('-');
	const abs = isNeg ? trimmed.slice(1) : trimmed;
	const { decimal, group } = amountSeparators(options.locale ?? getLocale());
	const parts = abs.split(decimal);
	if (parts.length > 2) return null;
	const [integer, fraction] = parts;
	if (fraction !== undefined && !/^[0-9]+$/.test(fraction)) return null;

	let intStr = integer;
	if (!/^[0-9]+$/.test(intStr)) {
		const groups = /^[ \u00a0\u202f]$/.test(group) ? [' ', '\u00a0', '\u202f'] : [group];
		const separator = groups.find(candidate => integer.includes(candidate));
		if (!separator) return null;
		const digits = integer.split(separator);
		if (
			!/^[0-9]{1,3}$/.test(digits[0]) ||
			!digits.slice(1).every(part => /^[0-9]{3}$/.test(part))
		) return null;
		intStr = digits.join('');
	}
	const fracStr = fraction ?? '';
	const scale = fracStr.length;

	const ceiling = Math.min(options.maxScale ?? MAX_SUPPORTED_SCALE, MAX_SUPPORTED_SCALE);
	if (scale > ceiling) return null;

	// Canonicalise to the minimal coefficient: strip leading zeros but keep one
	// digit, so "0.05" → value "5" scale 2 and "0.00" → value "0" scale 2.
	const coefficient = (intStr + fracStr).replace(/^0+/, '') || '0';
	if (coefficient.length > MAX_COEFFICIENT_DIGITS) return null;

	// A zero magnitude never carries a sign, so "-0.00" → "0".
	return { value: isNeg && coefficient !== '0' ? `-${coefficient}` : coefficient, scale };
}

/**
 * Render a ledger coefficient + scale as a plain decimal string.
 *
 * Locale-independent output for machine-readable values such as CSV. Editable
 * inputs use `formatEditableAmount`; read-only display belongs in `format.ts`.
 */
export function formatLedgerAmount(value: string, scale: number): string {
	const isNeg = value.startsWith('-');
	const abs = isNeg ? value.slice(1) : value;
	const sign = isNeg ? '-' : '';

	if (scale <= 0) return sign + abs;

	const padded = abs.padStart(scale + 1, '0');
	const int = padded.slice(0, padded.length - scale);
	const frac = padded.slice(padded.length - scale);
	return `${sign}${int}.${frac}`;
}

/** Exact editable decimal in the active locale, without grouping or rounding. */
export function formatEditableAmount(value: string, scale: number, locale = getLocale()): string {
	return formatLedgerAmount(value, scale).replace('.', amountSeparators(locale).decimal);
}

/**
 * Flip the sign of a coefficient or an ungrouped editable decimal string. A
 * zero magnitude stays unsigned, including decimal-comma inputs.
 */
export function negateCoefficient(value: string): string {
	if (!value) return '0';
	const isNeg = value.startsWith('-');
	const abs = isNeg ? value.slice(1) : value;
	if (/^0+(?:[.,]0+)?$/.test(abs)) return abs;
	return isNeg ? abs : `-${abs}`;
}

/** Sign of an exact integer coefficient without converting through Number. */
export function coefficientSign(value: string): -1 | 0 | 1 {
	const coefficient = BigInt(value);
	return coefficient < 0n ? -1 : coefficient > 0n ? 1 : 0;
}

/** The investment subledger still stores money coefficients in signed int64. */
export function fitsInt64Coefficient(value: string): boolean {
	const coefficient = BigInt(value);
	return coefficient >= -9223372036854775808n && coefficient <= 9223372036854775807n;
}

/**
 * Present a posting as an inflow-positive amount for the account it sits on.
 *
 * Storage is debit-positive (positive = debit). For liability, income, and
 * equity accounts a debit is a decrease, so the displayed sign is flipped to
 * match what a user means by "money in".
 */
export function inflowPositiveAmount(posting: {
	quantity_value: string;
	quantity_scale: number;
	account_class: string;
}): string {
	const raw = formatEditableAmount(posting.quantity_value, posting.quantity_scale);
	if (
		posting.account_class === 'liability' ||
		posting.account_class === 'income' ||
		posting.account_class === 'equity'
	) {
		return negateCoefficient(raw);
	}
	return raw;
}

/** Restate a coefficient at a larger scale. `to` must be >= `from`. */
function rescale(value: bigint, from: number, to: number): bigint {
	return value * 10n ** BigInt(to - from);
}

/** Exact sum of two amounts, aligned to the wider scale. */
export function addScaledAmounts(left: ScaledAmount, right: ScaledAmount): ScaledAmount {
	const scale = Math.max(left.scale, right.scale);
	const total = rescale(BigInt(left.value), left.scale, scale) + rescale(BigInt(right.value), right.scale, scale);
	return { value: total.toString(), scale };
}

/** Compare exact amounts with potentially different scales. */
export function compareScaledAmounts(left: ScaledAmount, right: ScaledAmount): -1 | 0 | 1 {
	const scale = Math.max(left.scale, right.scale);
	const a = rescale(BigInt(left.value), left.scale, scale);
	const b = rescale(BigInt(right.value), right.scale, scale);
	return a < b ? -1 : a > b ? 1 : 0;
}

/** One leg of a split, as the editor holds it before submission. */
export interface AmountLeg {
	commodityID: string;
	amountStr: string;
}

/**
 * Client-side balance hint for the split editor: the first commodity whose
 * legs do not sum to zero, or `null` when every commodity balances.
 *
 * Legs are summed **per commodity and scale-aware** — legs at different scales
 * are aligned to the widest scale before adding, which is the frontend
 * counterpart of `exact.ScaledInt` alignment. Unparseable and blank legs are
 * skipped; callers must refuse to submit those separately, since a leg this
 * function ignores is not a leg the backend will ignore.
 */
export function commodityImbalance(legs: AmountLeg[]): { commodityID: string; amount: string } | null {
	const totals = new Map<string, { total: bigint; scale: number }>();

	for (const leg of legs) {
		if (!leg.commodityID || !leg.amountStr.trim()) continue;
		const parsed = parseDecimalAmount(leg.amountStr);
		if (parsed === null) continue;

		const prev = totals.get(leg.commodityID);
		if (prev === undefined) {
			totals.set(leg.commodityID, { total: BigInt(parsed.value), scale: parsed.scale });
			continue;
		}

		const scale = Math.max(prev.scale, parsed.scale);
		totals.set(leg.commodityID, {
			total:
				rescale(prev.total, prev.scale, scale) +
				rescale(BigInt(parsed.value), parsed.scale, scale),
			scale
		});
	}

	for (const [commodityID, { total, scale }] of totals) {
		if (total !== 0n) {
			return { commodityID, amount: formatEditableAmount(total.toString(), scale) };
		}
	}
	return null;
}

/**
 * A posting reduced to the three fields any sign- or scale-aware operation
 * needs. Matches what `inflowPositiveAmount` already takes, plus the commodity
 * that groups it.
 */
export interface PostingAmount {
	commodity_id: number;
	quantity_value: string;
	quantity_scale: number;
	account_class: string;
}

/** One commodity's signed total, in the ledger's normal-sign convention. */
export interface CommodityTotal {
	commodityID: number;
	/** Signed coefficient string. Positive means "activity", per the rule below. */
	value: string;
	/** The widest scale any posting in this commodity carried. */
	scale: number;
}

/**
 * Sum postings per commodity, scale-aware, in the ledger's normal-sign
 * convention.
 *
 * The convention is the same one `inflowPositiveAmount` applies to a single
 * posting and `balanceMapToQuantities` applies on the backend: storage is
 * debit-positive, so a liability, income, or equity posting has its sign
 * flipped to make "money in" and "activity" read positive. Expense and asset
 * postings pass through unchanged.
 *
 * Legs at different scales are aligned to the widest scale before adding —
 * the frontend counterpart of `exact.ScaledInt` alignment, and the step whose
 * omission caused every historical severity-1 money bug in this repo. Nothing
 * is converted to a `Number`, so a total past `Number.MAX_SAFE_INTEGER` stays
 * exact.
 *
 * Commodities are returned in first-seen order, so a caller rendering them gets
 * a stable sequence rather than one that depends on map iteration.
 */
export function sumByCommodity(postings: PostingAmount[]): CommodityTotal[] {
	const totals = new Map<number, { total: bigint; scale: number }>();
	const order: number[] = [];

	for (const posting of postings) {
		const signed =
			posting.account_class === 'liability' ||
			posting.account_class === 'income' ||
			posting.account_class === 'equity'
				? -BigInt(posting.quantity_value)
				: BigInt(posting.quantity_value);

		const prev = totals.get(posting.commodity_id);
		if (prev === undefined) {
			order.push(posting.commodity_id);
			totals.set(posting.commodity_id, { total: signed, scale: posting.quantity_scale });
			continue;
		}

		const scale = Math.max(prev.scale, posting.quantity_scale);
		totals.set(posting.commodity_id, {
			total: rescale(prev.total, prev.scale, scale) + rescale(signed, posting.quantity_scale, scale),
			scale
		});
	}

	return order.map((commodityID) => {
		const { total, scale } = totals.get(commodityID)!;
		return { commodityID, value: total.toString(), scale };
	});
}

/** Display-only rounding: nearest unit at the requested scale, ties away from zero.
 * Never use this to allocate basis, persist amounts, or sum report rows. */
export function roundAmountForDisplay(value: string, scale: number, displayScale: number): string {
	const coefficient = BigInt(value);
	if (displayScale >= scale) return rescale(coefficient, scale, displayScale).toString();
	const divisor = 10n ** BigInt(scale - displayScale);
	const magnitude = coefficient < 0n ? -coefficient : coefficient;
	const rounded = (magnitude + divisor / 2n) / divisor;
	return (coefficient < 0n ? -rounded : rounded).toString();
}

/** Display-only division: `numerator / denominator` as a coefficient at
 * `resultScale`, rounded to the nearest unit with ties away from zero — the
 * same rule as the backend's `scaledDivision`. Used for implied prices such as
 * an FX rate stored as a price-per-quantity pair. Returns `null` when the
 * denominator is zero. Never use this to allocate, persist, or sum amounts. */
export function divideAmountsForDisplay(
	numerator: ScaledAmount,
	denominator: ScaledAmount,
	resultScale: number
): string | null {
	let n = BigInt(numerator.value);
	let d = BigInt(denominator.value);
	if (d === 0n) return null;
	const exponent = resultScale + denominator.scale - numerator.scale;
	if (exponent >= 0) n *= 10n ** BigInt(exponent);
	else d *= 10n ** BigInt(-exponent);
	const negative = n < 0n !== d < 0n;
	const absN = n < 0n ? -n : n;
	const absD = d < 0n ? -d : d;
	let quotient = absN / absD;
	if ((absN % absD) * 2n >= absD) quotient += 1n;
	return (negative && quotient !== 0n ? -quotient : quotient).toString();
}
