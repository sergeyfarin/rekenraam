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

/**
 * A well-formed decimal amount: an optional sign, an integer part that is
 * either plain digits or correctly grouped in threes, and an optional
 * fractional part.
 *
 * The grouping rule matters. The `en` locale this app ships treats "," as a
 * thousands separator, so "1,234.56" is 1234.56. Blindly stripping every comma
 * also turns the decimal-comma input "1,50" into 150 — a silent 100× error,
 * the same class of bug as T-36 on the import side. Requiring three-digit
 * groups makes "1,50" fail to parse so the form rejects it, rather than
 * guessing which separator the user meant. Real locale-aware input is a
 * separate change, tracked as G-08.
 */
const DECIMAL_AMOUNT = /^-?(?:\d+|\d{1,3}(?:,\d{3})+)(?:\.\d+)?$/;

/**
 * Parse a user-entered decimal string into a canonical coefficient + scale.
 *
 * The scale is the number of fractional digits the user actually typed; the
 * backend stores it as-is and does not require it to match the commodity's
 * standard scale. Returns `null` for anything that is not a valid signed
 * decimal so callers can reject the input instead of sending a malformed
 * amount.
 */
export function parseDecimalAmount(
	input: string,
	options: { maxScale?: number } = {}
): ScaledAmount | null {
	const trimmed = input.trim();
	if (!trimmed || !DECIMAL_AMOUNT.test(trimmed)) return null;

	const isNeg = trimmed.startsWith('-');
	const abs = (isNeg ? trimmed.slice(1) : trimmed).replace(/,/g, '');

	const dotIdx = abs.indexOf('.');
	const intStr = dotIdx === -1 ? abs : abs.slice(0, dotIdx);
	const fracStr = dotIdx === -1 ? '' : abs.slice(dotIdx + 1);
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
 * Deliberately not locale-formatted: this feeds editable form inputs, which
 * must round-trip back through `parseDecimalAmount` unchanged. Locale-aware
 * presentation of read-only money belongs in the Dinero.js display layer.
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

/**
 * Flip the sign of a coefficient or of a plain decimal string. A zero
 * magnitude stays unsigned, so "0.00" negates to "0.00", never "-0.00".
 */
export function negateCoefficient(value: string): string {
	if (!value) return '0';
	const isNeg = value.startsWith('-');
	const abs = isNeg ? value.slice(1) : value;
	if (/^0+$/.test(abs.replace('.', ''))) return abs;
	return isNeg ? abs : `-${abs}`;
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
	const raw = formatLedgerAmount(posting.quantity_value, posting.quantity_scale);
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
			return { commodityID, amount: formatLedgerAmount(total.toString(), scale) };
		}
	}
	return null;
}
