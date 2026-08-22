import { parseDecimalAmount, type ScaledAmount } from '$lib/money/amount';
import { toInt64Coefficient } from '$lib/api/investments';

/**
 * Submit-time amount validation for the three investment forms.
 *
 * This lives outside the `.svelte` files on purpose. The forms previously
 * carried seven private copies of three helpers — a `parseDecimalField` each,
 * a `toSafeInt` each, and a `rawLedgerToDisplay` in the dividend form — and
 * consolidating them onto `$lib/money` is a **behaviour change per form**, not
 * a like-for-like swap. There is no component-test harness in this project, so
 * the only way to pin that behaviour with named tests is for the validation to
 * be a plain module the component calls.
 *
 * ## The two behaviour changes
 *
 * 1. **Negative input is now rejected explicitly.** The old parsers rejected a
 *    leading `-` only as a side effect: they concatenated the integer and
 *    fractional digits and tested the result with `/^\d+$/`, so `-5.00` built
 *    the coefficient `"-500"` and failed the digit check. `parseDecimalAmount`
 *    handles signs correctly, so dropping it in without a guard would start
 *    accepting negative quantities and amounts. Every field here is a
 *    magnitude — you cannot buy -5 shares — so the guard is explicit and
 *    named, not implied by a regex.
 *
 * 2. **`1,50` no longer silently means 150.** The old parsers stripped every
 *    comma before parsing, so a decimal-comma amount became a 100× error — the
 *    same defect as T-45 on the editor side and T-36 on the import side.
 *    `parseDecimalAmount` requires commas to be correct thousands grouping and
 *    rejects `1,50` outright, so the user gets a rejection instead of a
 *    hundredfold overstatement. `1,234.56` still parses as 1234.56.
 *
 * No arithmetic happens here beyond what `$lib/money` does. Quantities travel
 * as exact coefficient strings; money fields additionally pass through
 * `toInt64Coefficient` because of the contract quirk documented there.
 */

/** Why a field was rejected. The component maps this to a translated message. */
export type AmountFieldError = 'invalid' | 'negative' | 'too_large';

/** A money field, carrying both the exact coefficient and its int64 form. */
export interface MoneyField extends ScaledAmount {
  /** The coefficient as the `number` this API's money fields are typed as. */
  int64: number;
}

export type FieldResult<T> = { ok: true; field: T } | { ok: false; reason: AmountFieldError };

/**
 * Parse a user-entered magnitude: a valid decimal that is not negative.
 *
 * Zero is allowed — a zero-amount dividend or a zero cash leg is a question for
 * the endpoint, not for a string parser, and the backend already rules on it.
 */
export function parseMagnitude(input: string, options: { maxScale?: number } = {}): FieldResult<ScaledAmount> {
  const parsed = parseDecimalAmount(input, options);
  if (parsed === null) {
    return { ok: false, reason: 'invalid' };
  }
  if (parsed.value.startsWith('-')) {
    return { ok: false, reason: 'negative' };
  }
  return { ok: true, field: parsed };
}

/**
 * Parse a magnitude destined for one of this API's `integer/int64` money
 * fields, rejecting values that a JS `number` cannot carry losslessly.
 */
export function parseMoneyMagnitude(input: string, options: { maxScale?: number } = {}): FieldResult<MoneyField> {
  const parsed = parseMagnitude(input, options);
  if (!parsed.ok) {
    return parsed;
  }
  const int64 = toInt64Coefficient(parsed.field.value);
  if (int64 === null) {
    return { ok: false, reason: 'too_large' };
  }
  return { ok: true, field: { ...parsed.field, int64 } };
}

/** Which field of a form failed, so the component can point at the right input. */
export type BuyField = 'quantity' | 'cash_amount';
export type SellField = 'quantity' | 'cash_amount';
export type DividendField = 'amount' | 'withholding' | 'quantity';

export type FormResult<TField, TValues> =
  | { ok: true; values: TValues }
  | { ok: false; field: TField; reason: AmountFieldError };

export interface TradeAmounts {
  quantity: ScaledAmount;
  cashAmount: MoneyField;
}

/**
 * Validate a buy or sell form's amounts.
 *
 * Both trades take the same two magnitudes, so they share a validator; the
 * distinct `BuyField`/`SellField` aliases keep each form's call site readable
 * and let the two diverge later without a rename.
 */
export function parseTradeAmounts(input: {
  quantityStr: string;
  cashAmountStr: string;
  quantityMaxScale?: number;
}): FormResult<BuyField, TradeAmounts> {
  const quantity = parseMagnitude(input.quantityStr, { maxScale: input.quantityMaxScale });
  if (!quantity.ok) {
    return { ok: false, field: 'quantity', reason: quantity.reason };
  }
  const cashAmount = parseMoneyMagnitude(input.cashAmountStr);
  if (!cashAmount.ok) {
    return { ok: false, field: 'cash_amount', reason: cashAmount.reason };
  }
  return { ok: true, values: { quantity: quantity.field, cashAmount: cashAmount.field } };
}

export interface DividendAmounts {
  amount: MoneyField;
  /** Present only when the user opened the withholding section and filled it. */
  withholding: MoneyField | null;
  /** Present only in reinvested mode. */
  quantity: ScaledAmount | null;
}

/**
 * Validate a dividend form's amounts, in either cash or reinvested mode.
 *
 * Withholding is optional: a blank field is `null`, not an error. Quantity is
 * required in reinvested mode and absent in cash mode.
 */
export function parseDividendAmounts(input: {
  amountStr: string;
  withholdingStr: string;
  includeWithholding: boolean;
  quantityStr: string;
  includeQuantity: boolean;
  quantityMaxScale?: number;
}): FormResult<DividendField, DividendAmounts> {
  const amount = parseMoneyMagnitude(input.amountStr);
  if (!amount.ok) {
    return { ok: false, field: 'amount', reason: amount.reason };
  }

  let withholding: MoneyField | null = null;
  if (input.includeWithholding && input.withholdingStr.trim() !== '') {
    const parsed = parseMoneyMagnitude(input.withholdingStr);
    if (!parsed.ok) {
      return { ok: false, field: 'withholding', reason: parsed.reason };
    }
    withholding = parsed.field;
  }

  let quantity: ScaledAmount | null = null;
  if (input.includeQuantity) {
    const parsed = parseMagnitude(input.quantityStr, { maxScale: input.quantityMaxScale });
    if (!parsed.ok) {
      return { ok: false, field: 'quantity', reason: parsed.reason };
    }
    quantity = parsed.field;
  }

  return { ok: true, values: { amount: amount.field, withholding, quantity } };
}
