import { apiGetWithTauriFallback } from "$lib/api/client";

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

export async function listAccountTree(bookId = 1): Promise<AccountTreeNode[]> {
  return apiGetWithTauriFallback<AccountTreeNode[]>(`/accounts/tree`, "get_account_tree", { bookId });
}

export async function getAccountById(accountId: number): Promise<AccountSummary | null> {
  try {
    return await apiGetWithTauriFallback<AccountSummary>(`/accounts/${accountId}`, "get_account", { id: accountId });
  } catch (error) {
    if (error instanceof Error && error.message.includes("account not found")) {
      return null;
    }
    throw error;
  }
}