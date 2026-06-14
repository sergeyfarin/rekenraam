import type { AccountResponse } from '$lib/api/accounts';
import type { InstitutionResponse } from '$lib/api/institutions';
import type { CurrencyResponse } from '$lib/api/currencies';
import { m } from '$lib/paraglide/messages.js';

export type AccountClass = AccountResponse['account_class'];
export type ManagedAccountClass = Exclude<AccountClass, 'income' | 'expense'>;
export type AccountKind = AccountResponse['account_kind'];
export type AccountStatus = AccountResponse['status'];
export type InstitutionKind = InstitutionResponse['kind'];

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
    case 'term_deposit':
      return m.account_kind_term_deposit();
    case 'brokerage':
      return m.account_kind_brokerage();
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
    case 'rewards_balance':
      return m.account_kind_rewards_balance();
    case 'receivable':
      return m.account_kind_receivable();
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
    case 'equity':
      return m.account_kind_equity();
    case 'income':
      return m.account_kind_income();
    case 'expense':
      return m.account_kind_expense();
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

export function isCategoryAccount(account: AccountResponse): boolean {
  return account.account_class === 'income' || account.account_class === 'expense';
}

export function accountDisplayName(account: AccountResponse): string {
  const starterName = starterAccountDisplayName(account);
  if (starterName) {
    return starterName;
  }

  return account.name?.trim() || account.code?.trim() || m.accounts_unnamed_account({ id: account.id });
}

function starterAccountDisplayName(account: AccountResponse): string {
  if (account.name?.trim()) {
    return '';
  }

  const starterKey = account.metadata['starter_key'];
  const currencyCode = account.metadata['currency_code'];
  if (starterKey === 'cash' && typeof currencyCode === 'string' && currencyCode.trim() !== '') {
    return m.accounts_starter_cash_name({ currency: currencyCode.trim() });
  }

  return '';
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

export function institutionKindLabel(value: InstitutionKind): string {
  switch (value) {
    case 'bank':
      return m.institution_kind_bank();
    case 'credit_union':
      return m.institution_kind_credit_union();
    case 'brokerage':
      return m.institution_kind_brokerage();
    case 'card_issuer':
      return m.institution_kind_card_issuer();
    case 'lender':
      return m.institution_kind_lender();
    case 'insurance':
      return m.institution_kind_insurance();
    case 'employer':
      return m.institution_kind_employer();
    case 'rewards_program':
      return m.institution_kind_rewards_program();
    case 'government':
      return m.institution_kind_government();
    case 'other':
      return m.institution_kind_other();
  }
}

export function institutionCountryLabel(institution: InstitutionResponse, countryNames: Intl.DisplayNames): string {
  if (!institution.country_code) {
    return m.accounts_group_no_country();
  }

  return countryNames.of(institution.country_code) ?? institution.country_code;
}
