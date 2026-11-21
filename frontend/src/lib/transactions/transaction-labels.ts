import type { components } from '#lib/api/schema.js';
import { m } from '#lib/paraglide/messages.js';
import { accountLabelFromParts } from '#lib/accounts/account-labels.ts';
import { formatQuantity } from '#lib/money/format.ts';

export type PostingResponse = components['schemas']['PostingResponse'];
export type TransactionStatus = components['schemas']['TransactionStatus'];
export type AccountClass = components['schemas']['AccountClass'];
export type StatusBadgeTone = 'neutral' | 'accent' | 'danger' | 'positive' | 'warning';

/**
 * Resolve the display label of a posting's account: a system
 * account by its localized role, anything else through the shared account
 * label, which keeps the owner's name first (a renamed starter category keeps
 * its name, though its built-in key survives the rename), then the built-in
 * category label, then a starter account's localized label such as "Cash
 * USD", then the code, then "Account N".
 *
 * Never returns null/undefined — always a non-empty string.
 */
export function resolveAccountLabel(account: PostingResponse): string {
  if (account.account_system_role) {
    return systemRoleLabel(account.account_system_role);
  }
  return accountLabelFromParts({ id: account.account_id, name: account.account_name, code: account.account_code,
    builtinKey: account.account_builtin_key, starterKey: account.account_starter_key, currencyCode: account.account_currency_code });
}

export function systemRoleLabel(role: string): string {
  switch (role) {
    case 'import_imbalance': return m.account_system_role_import_imbalance();
    case 'retained_earnings': return m.account_system_role_retained_earnings();
    case 'unassigned_income': return m.account_system_role_unassigned_income();
    case 'unassigned_expense': return m.account_system_role_unassigned_expense();
    case 'commodity_trading': return m.account_system_role_commodity_trading();
    case 'external_investment_transfer_equity': return m.account_system_role_external_investment_transfer_equity();
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
