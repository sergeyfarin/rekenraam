import type { components } from '$lib/api/schema';
import { m } from '$lib/paraglide/messages.js';
import { builtinCategoryLabel } from '$lib/categories/category-labels';
import { formatQuantity } from '$lib/money/format';

export type PostingResponse = components['schemas']['PostingResponse'];
export type TransactionStatus = components['schemas']['TransactionStatus'];
export type AccountClass = components['schemas']['AccountClass'];
export type StatusBadgeTone = 'neutral' | 'accent' | 'danger' | 'positive' | 'warning';

/**
 * Resolve a posting's account display label using the priority chain:
 * account_system_role (localised) → account_builtin_key (localised) →
 * account_name → account_code → fallback.
 *
 * Never returns null/undefined — always a non-empty string.
 */
export function resolveAccountLabel(posting: PostingResponse): string {
  if (posting.account_system_role) {
    return systemRoleLabel(posting.account_system_role);
  }
  if (posting.account_builtin_key) {
    return builtinCategoryLabel(posting.account_builtin_key);
  }
  if (posting.account_name) {
    return posting.account_name;
  }
  if (posting.account_code) {
    return posting.account_code;
  }
  return `#${posting.account_id}`;
}

function systemRoleLabel(role: string): string {
  switch (role) {
    case 'transfer_clearing':
      return m.account_system_role_transfer_clearing();
    case 'opening_balance':
      return m.account_system_role_opening_balance();
    default:
      return role;
  }
}

export function statusLabel(status: TransactionStatus): string {
  switch (status) {
    case 'posted':
      return m.transaction_status_posted();
    case 'voided':
      return m.transaction_status_voided();
    case 'draft':
      return m.transaction_status_draft();
  }
}

export function statusTone(status: TransactionStatus): StatusBadgeTone {
  switch (status) {
    case 'posted':
      return 'accent';
    case 'voided':
      return 'warning';
    case 'draft':
      return 'neutral';
  }
}

/**
 * Sign convention table (inflow positive, outflow negative from the account perspective):
 *
 * | Account class | Debit (+qty_value) | Credit (-qty_value) |
 * |---------------|-------------------|---------------------|
 * | asset         | Inflow  (+)        | Outflow (−)         |
 * | liability     | Outflow (−)        | Inflow  (+)         |
 * | income        | Outflow (−)        | Inflow  (+)         |
 * | expense       | Inflow  (+)        | Outflow (−)         |
 * | equity        | Outflow (−)        | Inflow  (+)         |
 *
 * quantity_value is the raw ledger coefficient. A positive coefficient is a debit;
 * a negative coefficient is a credit (standard double-entry convention).
 *
 * perspectiveClass controls sign inversion: asset/expense → positive debit is inflow;
 * liability/income/equity → positive debit is outflow (flip).
 */
export function formatSignedAmount(
  posting: PostingResponse,
  perspectiveClass: AccountClass,
  locale: string
): string {
  const base = formatQuantity(posting.quantity_value, posting.quantity_scale, locale);
  if (!base) return '';

  const isNegativeCoefficient = posting.quantity_value.startsWith('-');

  // For liability/income/equity: flip the sign (positive debit = outflow = display negative).
  const shouldFlip = perspectiveClass === 'liability' || perspectiveClass === 'income' || perspectiveClass === 'equity';

  const displayNegative = shouldFlip ? !isNegativeCoefficient : isNegativeCoefficient;

  // base already includes its own sign from formatQuantity — strip it and reapply.
  const absBase = base.startsWith('-') ? base.slice(1) : base;
  return displayNegative ? `-${absBase}` : absBase;
}

/**
 * Returns the commodity display prefix: symbol if available, else commodity_code.
 * E.g. "$" or "EUR".
 */
export function commodityDisplay(posting: PostingResponse): string {
  return posting.commodity_symbol ?? posting.commodity_code;
}
