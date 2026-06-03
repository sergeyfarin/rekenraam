import type { AccountResponse } from '$lib/api/accounts';
import type { InstitutionResponse } from '$lib/api/institutions';
import type { CurrencyResponse } from '$lib/api/currencies';
import { m } from '$lib/paraglide/messages.js';

export type AccountClass = AccountResponse['account_class'];
export type AccountKind = AccountResponse['account_kind'];
export type AccountStatus = AccountResponse['status'];

export function accountClassLabel(accountClass: AccountClass): string {
  switch (accountClass) {
    case 'asset':
      return m.account_class_asset();
    case 'liability':
      return m.account_class_liability();
    case 'equity':
      return m.account_class_equity();
    case 'income':
      return m.account_class_income();
    case 'expense':
      return m.account_class_expense();
  }
}

export function accountClassRank(accountClass: AccountClass): number {
  switch (accountClass) {
    case 'asset':
      return 1;
    case 'liability':
      return 2;
    case 'equity':
      return 3;
    case 'income':
      return 4;
    case 'expense':
      return 5;
  }
}

export function accountKindLabel(accountKind: AccountKind): string {
  switch (accountKind) {
    case 'cash':
      return m.account_kind_cash();
    case 'checking':
      return m.account_kind_checking();
    case 'savings':
      return m.account_kind_savings();
    case 'time_deposit':
      return m.account_kind_time_deposit();
    case 'money_market':
      return m.account_kind_money_market();
    case 'investment':
      return m.account_kind_investment();
    case 'brokerage_cash':
      return m.account_kind_brokerage_cash();
    case 'security_holding':
      return m.account_kind_security_holding();
    case 'crypto_wallet':
      return m.account_kind_crypto_wallet();
    case 'property':
      return m.account_kind_property();
    case 'vehicle':
      return m.account_kind_vehicle();
    case 'collectible':
      return m.account_kind_collectible();
    case 'points_miles':
      return m.account_kind_points_miles();
    case 'loan_receivable':
      return m.account_kind_loan_receivable();
    case 'other_asset':
      return m.account_kind_other_asset();
    case 'credit_card':
      return m.account_kind_credit_card();
    case 'line_of_credit':
      return m.account_kind_line_of_credit();
    case 'loan':
      return m.account_kind_loan();
    case 'mortgage':
      return m.account_kind_mortgage();
    case 'tax_liability':
      return m.account_kind_tax_liability();
    case 'payable':
      return m.account_kind_payable();
    case 'other_liability':
      return m.account_kind_other_liability();
    case 'opening_balance':
      return m.account_kind_opening_balance();
    case 'retained_earnings':
      return m.account_kind_retained_earnings();
    case 'current_earnings':
      return m.account_kind_current_earnings();
    case 'trading':
      return m.account_kind_trading();
    case 'imbalance':
      return m.account_kind_imbalance();
    case 'equity':
      return m.account_kind_equity();
    case 'salary':
      return m.account_kind_salary();
    case 'interest':
      return m.account_kind_interest();
    case 'dividend':
      return m.account_kind_dividend();
    case 'realized_capital_gain':
      return m.account_kind_realized_capital_gain();
    case 'unrealized_capital_gain':
      return m.account_kind_unrealized_capital_gain();
    case 'reward_income':
      return m.account_kind_reward_income();
    case 'other_income':
      return m.account_kind_other_income();
    case 'expense':
      return m.account_kind_expense();
    case 'fee':
      return m.account_kind_fee();
    case 'tax':
      return m.account_kind_tax();
    case 'interest_expense':
      return m.account_kind_interest_expense();
    case 'investment_fee':
      return m.account_kind_investment_fee();
    case 'other_expense':
      return m.account_kind_other_expense();
    case 'realized_losses':
      return m.account_kind_realized_losses();
    case 'unrealized_losses':
      return m.account_kind_unrealized_losses();
  }
}

export function accountStatusLabel(status: AccountStatus): string {
  switch (status) {
    case 'active':
      return m.account_status_active();
    case 'closed':
      return m.account_status_closed();
    case 'archived':
      return m.account_status_archived();
  }
}

export function accountDisplayName(account: AccountResponse): string {
  return account.name?.trim() || account.code?.trim() || m.accounts_unnamed_account({ id: account.id });
}

export function accountInstitutionLabel(
  account: AccountResponse,
  institutionsByID: ReadonlyMap<number, InstitutionResponse>
): string {
  if (!account.institution_id) {
    return m.accounts_group_no_institution();
  }

  return institutionsByID.get(account.institution_id)?.name ?? m.accounts_group_unknown_institution();
}

export function accountCountryLabel(account: AccountResponse, countryNames: Intl.DisplayNames): string {
  if (!account.country_code) {
    return m.accounts_group_no_country();
  }

  return countryNames.of(account.country_code) ?? account.country_code;
}

export function accountCommodityLabel(
  account: AccountResponse,
  currenciesByID: ReadonlyMap<number, CurrencyResponse>
): string {
  if (!account.default_commodity_id) {
    return m.accounts_no_default_commodity();
  }

  const currency = currenciesByID.get(account.default_commodity_id);
  if (!currency) {
    return m.accounts_unknown_commodity();
  }

  return `${currency.code} ${currency.display_symbol}`;
}
