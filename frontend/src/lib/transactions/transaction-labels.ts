import type { components } from '#lib/api/schema.js';
import { m } from '#lib/paraglide/messages.js';
import { accountLabelFromParts } from '#lib/accounts/account-labels.ts';
import { accountNormalCoefficient } from '#lib/money/amount.ts';
import { formatQuantity } from '#lib/money/format.ts';

export type PostingResponse = components['schemas']['PostingResponse'];
export type TransactionStatus = components['schemas']['TransactionStatus'];
export type AccountClass = components['schemas']['AccountClass'];
export type StatusBadgeTone = 'neutral' | 'accent' | 'danger' | 'positive' | 'warning';

/** The account label parts a posting or a reconciliation checkpoint carries. */
export type AccountLabelParts = Pick<PostingResponse, 'account_id' | 'account_name' | 'account_code' | 'account_system_role'
  | 'account_builtin_key' | 'account_starter_key' | 'account_currency_code'>;

/**
 * Resolve the display label of a posting's or a checkpoint's account: a system
 * account by its localized role, anything else through the shared account
 * label, which keeps the owner's name first (a renamed starter category keeps
 * its name, though its built-in key survives the rename), then the built-in
 * category label, then a starter account's localized label such as "Cash
 * USD", then the code, then "Account N".
 *
 * Never returns null/undefined — always a non-empty string.
 */
export function resolveAccountLabel(account: AccountLabelParts): string {
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

/** Format a debit-positive posting from the given account's perspective. */
export function formatSignedAmount(
  posting: Pick<PostingResponse, 'quantity_value' | 'quantity_scale'>,
  perspectiveClass: AccountClass,
  locale: string
): string {
  const value = accountNormalCoefficient(posting.quantity_value, perspectiveClass);
  return formatQuantity(value, posting.quantity_scale, locale);
}

/**
 * Returns the commodity display prefix: symbol if available, else commodity_code.
 * E.g. "$" or "EUR".
 */
export function commodityDisplay(posting: PostingResponse): string {
  return posting.commodity_symbol ?? posting.commodity_code;
}
