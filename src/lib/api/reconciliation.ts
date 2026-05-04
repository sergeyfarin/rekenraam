import { apiDelete, apiGet, apiPost } from "$lib/api/client";
import type { AccountBalancingSummary, AccountSummary } from "$lib/api/accounts";
import type { TransactionWithSplits } from "$lib/api/transactions";

export type ReconciliationCommoditySummary = {
  id: number;
  name: string;
  symbol: string | null;
  scale: number;
};

export type BalanceCheckSummary = {
  id: number;
  book_id: number;
  account_id: number;
  as_of_date: string;
  balance_minor: number;
  memo: string | null;
  status: string;
  created_at: string;
};

export type ReconciliationBalancingSummary = {
  id: number;
  book_id: number;
  account_id: number;
  as_of_date: string;
  balance_minor: number;
  memo: string | null;
  created_at: string;
  voided_at: string | null;
  void_reason: string | null;
};

export type BalanceAdjustmentSummary = {
  id: number;
  book_id: number;
  balance_check_id: number | null;
  account_id: number;
  offset_account_id: number;
  as_of_date: string;
  target_balance_minor: number;
  actual_balance_minor: number;
  adjustment_minor: number;
  tx_id: number;
  mark: string;
  memo: string | null;
  created_at: string;
};

export type ReconciliationStartResponse = {
  account: AccountSummary;
  commodity: ReconciliationCommoditySummary;
  last_balancing: AccountBalancingSummary | null;
  statement_start_date: string | null;
  statement_end_date: string;
  remembered_offset_account_id: number | null;
  candidates: TransactionWithSplits[];
};

type ApiReconciliationStartResponse = Omit<ReconciliationStartResponse, "candidates"> & {
  candidates: ApiTransaction[];
};

export type ReconciliationFinishInput = {
  book_id: number;
  as_of_date: string;
  statement_start_date: string | null;
  statement_balance_minor: number;
  selected_transaction_ids: number[];
  offset_account_id: number | null;
  remember_offset_account: boolean;
  memo: string | null;
  confirm_unbalanced: boolean;
  downgrade_unselected_cleared: boolean;
};

export type ReconciliationFinishResult = {
  balance_check: BalanceCheckSummary;
  balancing: AccountBalancingSummary;
  adjustment: BalanceAdjustmentSummary | null;
  difference_minor: number;
  reconciled_count: number;
  uncleared_count: number;
};

export type ReconciliationHistoryResponse = {
  balancings: ReconciliationBalancingSummary[];
  balance_checks: BalanceCheckSummary[];
  balance_adjustments: BalanceAdjustmentSummary[];
};

export type BalanceConstraintSummary = {
  id: number;
  book_id: number;
  account_id: number;
  min_balance_minor: number | null;
  max_balance_minor: number | null;
  sign_rule: string;
  created_at: string;
  updated_at: string;
};

type ApiSplit = {
  id: number;
  tx_id: number;
  account_id: number;
  commodity_id: number;
  amount_minor: number;
  category_id: number | null;
  tag_id: number | null;
  person_id: number | null;
  project_id: number | null;
  share_bps: number | null;
  memo: string | null;
};

type ApiTransaction = {
  id: number;
  book_id: number;
  occurred_date: string;
  posted_date: string;
  payee_id: number | null;
  memo: string | null;
  status: string;
  reference: string | null;
  created_at: string;
  splits: ApiSplit[];
};

function mapTransaction(transaction: ApiTransaction): TransactionWithSplits {
  return {
    transaction: {
      id: transaction.id,
      txn_date: transaction.occurred_date,
      payee_id: transaction.payee_id,
      memo: transaction.memo,
      status: transaction.status,
      reference: transaction.reference,
    },
    splits: transaction.splits.map((split) => ({
      id: split.id,
      tx_id: split.tx_id,
      account_id: split.account_id,
      commodity_id: split.commodity_id,
      amount_minor: split.amount_minor,
      category_id: split.category_id,
      tag_id: split.tag_id,
      person_id: split.person_id,
      project_id: split.project_id,
      share_bps: split.share_bps,
      memo: split.memo,
    })),
  };
}

export async function startReconciliation(
  accountId: number,
  asOfDate: string,
  statementStartDate: string | null = null,
): Promise<ReconciliationStartResponse> {
  const params = new URLSearchParams({ as_of_date: asOfDate });
  if (statementStartDate) params.set("statement_start_date", statementStartDate);
  const response = await apiGet<ApiReconciliationStartResponse>(`/reconciliation/accounts/${accountId}/start?${params.toString()}`);
  return {
    ...response,
    candidates: response.candidates.map(mapTransaction),
  };
}

export async function finishReconciliation(
  accountId: number,
  input: ReconciliationFinishInput,
): Promise<ReconciliationFinishResult> {
  return apiPost<ReconciliationFinishResult, ReconciliationFinishInput>(
    `/reconciliation/accounts/${accountId}/finish`,
    input,
  );
}

export async function getReconciliationHistory(accountId: number): Promise<ReconciliationHistoryResponse> {
  return apiGet<ReconciliationHistoryResponse>(`/reconciliation/accounts/${accountId}/history`);
}

export async function unlockReconciliation(
  accountId: number,
  fromDate: string,
  reason: string | null,
  confirm: boolean,
): Promise<number> {
  return apiPost<number, { from_date: string; reason: string | null; confirm: boolean }>(
    `/reconciliation/accounts/${accountId}/unlock`,
    { from_date: fromDate, reason, confirm },
  );
}

export async function listBalanceConstraints(accountId: number): Promise<BalanceConstraintSummary[]> {
  return apiGet<BalanceConstraintSummary[]>(`/reconciliation/accounts/${accountId}/constraints`);
}

export async function createBalanceConstraint(
  accountId: number,
  input: { book_id: number; min_balance_minor: number | null; max_balance_minor: number | null; sign_rule: string },
): Promise<BalanceConstraintSummary> {
  return apiPost<BalanceConstraintSummary, typeof input>(`/reconciliation/accounts/${accountId}/constraints`, input);
}

export async function deleteBalanceConstraint(accountId: number, constraintId: number): Promise<void> {
  await apiDelete<void>(`/reconciliation/accounts/${accountId}/constraints/${constraintId}`);
}
