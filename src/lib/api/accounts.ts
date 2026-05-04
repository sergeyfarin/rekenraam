import { apiDelete, apiGet, apiPost, apiPut } from "$lib/api/client";

export type AccountTreeNode = {
  id: number;
  parent_id: number | null;
  name: string;
  account_type: string;
  commodity_id: number;
  commodity_name: string;
  commodity_scale: number;
  institution_name: string | null;
  country_name: string | null;
  balance_minor: number;
  rollup_balance_minor: number;
  children: AccountTreeNode[];
};

export type AccountSummary = {
  id: number;
  book_id: number;
  parent_id: number | null;
  account_type: string;
  name: string;
  commodity_id: number;
  institution_id: number | null;
  institution_name: string | null;
  country_id: number | null;
  country_name: string | null;
  number_last4: string | null;
  is_closed: boolean;
  is_hidden: boolean;
  is_system: boolean;
  system_role: string | null;
  created_at: string;
  updated_at: string;
};

export type AccountBalanceSummary = {
  account_id: number;
  balance_minor: number;
};

export type AccountBalancingSummary = {
  id: number;
  account_id: number;
  as_of_date: string;
  balance_minor: number;
  memo: string | null;
};

export type AccountDirectiveSummary = {
  id: number;
  book_id: number;
  account_id: number;
  directive_type: string;
  directive_date: string;
  note: string | null;
  metadata: string | null;
  created_at: string;
};

export type AccountCreateInput = {
  book_id: number;
  parent_id: number | null;
  account_type: string;
  name: string;
  commodity_id: number;
  institution_id: number | null;
  country_id: number | null;
  number_last4: string | null;
  is_closed: boolean;
};

export type AccountClosingValidationResult = {
  valid: boolean;
  issues: string[];
};

export async function listAccounts(bookId = 1): Promise<AccountSummary[]> {
  void bookId;
  return apiGet<AccountSummary[]>(`/accounts`);
}

export async function createAccount(input: AccountCreateInput): Promise<AccountSummary> {
  return apiPost<AccountSummary, AccountCreateInput>("/accounts", input);
}

export async function updateAccount(input: AccountCreateInput & { id: number }): Promise<AccountSummary> {
  return apiPut<AccountSummary, AccountCreateInput>(`/accounts/${input.id}`, input);
}

export async function deleteAccount(accountId: number, bookId = 1): Promise<void> {
  void bookId;
  await apiDelete<void>(`/accounts/${accountId}`);
}

export async function validateAccountClosing(accountId: number, bookId = 1): Promise<AccountClosingValidationResult> {
  void bookId;
  return apiGet<AccountClosingValidationResult>(`/accounts/${accountId}/closing-validation`);
}

export async function listAccountTree(bookId = 1): Promise<AccountTreeNode[]> {
  void bookId;
  return apiGet<AccountTreeNode[]>(`/accounts/tree`);
}

export async function listAccountBalances(bookId = 1): Promise<AccountBalanceSummary[]> {
  void bookId;
  return apiGet<AccountBalanceSummary[]>(`/accounts/balances`);
}

export async function getAccountById(accountId: number): Promise<AccountSummary | null> {
  try {
    return await apiGet<AccountSummary>(`/accounts/${accountId}`);
  } catch (error) {
    if (error instanceof Error && error.message.includes("account not found")) {
      return null;
    }
    throw error;
  }
}

export async function listAccountBalancings(accountId: number): Promise<AccountBalancingSummary[]> {
  return apiGet<AccountBalancingSummary[]>(`/accounts/${accountId}/balancings`);
}

export async function listAccountDirectives(accountId: number): Promise<AccountDirectiveSummary[]> {
  return apiGet<AccountDirectiveSummary[]>(`/accounts/${accountId}/directives`);
}

export async function getAccountBookingPolicy(accountId: number): Promise<string> {
  return apiGet<string>(`/accounts/${accountId}/booking-policy`);
}

export async function setAccountBookingPolicy(accountId: number, bookingPolicy: string): Promise<string> {
  return apiPut<string, { booking_policy: string }>(
    `/accounts/${accountId}/booking-policy`,
    { booking_policy: bookingPolicy }
  );
}

export async function unlockAccountBalancings(
  accountId: number,
  fromDate: string,
  reason: string | null,
  confirm: boolean,
): Promise<number> {
  return apiPost<number, { from_date: string; reason: string | null; confirm: boolean }>(
    `/accounts/${accountId}/balancings/unlock`,
    { from_date: fromDate, reason, confirm },
  );
}

export async function createAccountBalancing(input: {
  book_id: number;
  account_id: number;
  as_of_date: string;
  balance_minor: number;
  memo: string | null;
}): Promise<AccountBalancingSummary | void> {
  return apiPost<AccountBalancingSummary, typeof input>(`/accounts/${input.account_id}/balancings`, input);
}