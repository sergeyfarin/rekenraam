import { apiDelete, apiGet, apiPost, apiPut } from "$lib/api/client";
import type { TransactionMutationInput } from "$lib/api/transactions";

function query(params: Record<string, string | number | null | undefined>): string {
  const searchParams = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== null && value !== undefined) searchParams.set(key, String(value));
  }
  const value = searchParams.toString();
  return value ? `?${value}` : "";
}

export type BudgetTargetInput = {
  category_id: number;
  amount_minor: number;
  rollover_enabled: boolean;
};

export type BudgetInput = {
  book_id: number;
  name: string;
  period: "monthly" | "annual";
  starts_on: string;
  ends_on: string | null;
  commodity_id: number;
  is_active: boolean;
  targets: BudgetTargetInput[];
};

export type Budget = BudgetInput & {
  id: number;
  created_at: string;
  targets: (BudgetTargetInput & { id: number; budget_id: number })[];
};

export type BudgetVarianceRow = {
  category_id: number;
  target_minor: number;
  actual_minor: number;
  rollover_minor: number;
  remaining_minor: number;
};

export type ScheduleSplitInput = {
  account_id: number;
  commodity_id: number;
  amount_minor: number;
  category_id: number | null;
  memo: string | null;
};

export type ScheduleInput = {
  book_id: number;
  name: string;
  payee_id: number | null;
  memo: string | null;
  status: "uncleared" | "cleared";
  reference: string | null;
  frequency: "daily" | "weekly" | "monthly" | "yearly";
  interval: number;
  start_date: string;
  end_date: string | null;
  reminder_days: number;
  enabled: boolean;
  splits: ScheduleSplitInput[];
};

export type Schedule = ScheduleInput & {
  id: number;
  splits: (ScheduleSplitInput & { id: number })[];
};

export type ScheduleOccurrence = {
  schedule_id: number;
  occurrence_date: string;
  status: "pending" | "skipped" | "posted";
  posted_transaction_id: number | null;
};

export type PostedScheduleResult = {
  occurrence: ScheduleOccurrence;
  transaction: TransactionSummary;
};

export type TransactionSummary = {
  id: number;
  book_id: number;
  previous_tx_id: number | null;
  occurred_date: string;
  posted_date: string;
  payee_id: number | null;
  memo: string | null;
  status: string;
  reference: string | null;
  import_id: string | null;
  created_at: string;
  splits: {
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
    created_at: string;
  }[];
};

export type ProjectedCashRow = {
  account_id: number;
  projection_date: string;
  scheduled_delta_minor: number;
  projected_balance_minor: number;
};

export type LoanInput = {
  book_id: number;
  account_id: number;
  name: string;
  principal_minor: number;
  annual_rate_bps: number;
  term_months: number;
  payment_frequency: "monthly";
  start_date: string;
  payment_account_id: number | null;
  interest_category_id: number | null;
};

export type Loan = LoanInput & {
  id: number;
  payment_minor: number;
};

export type AmortizationRow = {
  period: number;
  payment_date: string;
  payment_minor: number;
  principal_minor: number;
  interest_minor: number;
  remaining_principal_minor: number;
};

export type LoanPaymentDraftInput = {
  payment_date: string;
  payment_account_id: number | null;
  interest_category_id: number | null;
  memo: string | null;
};

export type LoanPaymentDraft = {
  transaction: TransactionMutationInput;
};

export async function listBudgets(bookId = 1): Promise<Budget[]> {
  return apiGet<Budget[]>(`/budgets${query({ book_id: bookId })}`);
}

export async function createBudget(input: BudgetInput): Promise<Budget> {
  return apiPost<Budget, BudgetInput>("/budgets", input);
}

export async function updateBudget(id: number, input: BudgetInput): Promise<Budget> {
  return apiPut<Budget, BudgetInput>(`/budgets/${id}`, input);
}

export async function deleteBudget(id: number): Promise<void> {
  return apiDelete<void>(`/budgets/${id}`);
}

export async function budgetVariance(id: number, periodStart: string): Promise<BudgetVarianceRow[]> {
  return apiGet<BudgetVarianceRow[]>(`/budgets/${id}/variance${query({ period_start: periodStart })}`);
}

export async function listSchedules(bookId = 1): Promise<Schedule[]> {
  return apiGet<Schedule[]>(`/schedules${query({ book_id: bookId })}`);
}

export async function createSchedule(input: ScheduleInput): Promise<Schedule> {
  return apiPost<Schedule, ScheduleInput>("/schedules", input);
}

export async function projectedInstances(bookId: number, start: string, end: string): Promise<ScheduleOccurrence[]> {
  return apiGet<ScheduleOccurrence[]>(`/schedules/instances${query({ book_id: bookId, start, end })}`);
}

export async function skipSchedule(id: number, occurrenceDate: string): Promise<ScheduleOccurrence> {
  return apiPost<ScheduleOccurrence, Record<string, never>>(`/schedules/${id}/skip${query({ occurrence_date: occurrenceDate })}`, {});
}

export async function updateSchedule(id: number, input: ScheduleInput): Promise<Schedule> {
  return apiPut<Schedule, ScheduleInput>(`/schedules/${id}`, input);
}

export async function deleteSchedule(id: number): Promise<void> {
  return apiDelete<void>(`/schedules/${id}`);
}

export async function postSchedule(id: number, occurrenceDate: string): Promise<PostedScheduleResult> {
  return apiPost<PostedScheduleResult, Record<string, never>>(`/schedules/${id}/post${query({ occurrence_date: occurrenceDate })}`, {});
}

export async function projectedCash(bookId: number, start: string, end: string): Promise<ProjectedCashRow[]> {
  return apiGet<ProjectedCashRow[]>(`/schedules/projected-cash${query({ book_id: bookId, start, end })}`);
}

export async function listLoans(bookId = 1): Promise<Loan[]> {
  return apiGet<Loan[]>(`/loans${query({ book_id: bookId })}`);
}

export async function createLoan(input: LoanInput): Promise<Loan> {
  return apiPost<Loan, LoanInput>("/loans", input);
}

export async function updateLoan(id: number, input: LoanInput): Promise<Loan> {
  return apiPut<Loan, LoanInput>(`/loans/${id}`, input);
}

export async function deleteLoan(id: number): Promise<void> {
  return apiDelete<void>(`/loans/${id}`);
}

export async function loanAmortization(id: number): Promise<AmortizationRow[]> {
  return apiGet<AmortizationRow[]>(`/loans/${id}/amortization`);
}

export async function loanPaymentDraft(id: number, input: LoanPaymentDraftInput): Promise<LoanPaymentDraft> {
  return apiPost<LoanPaymentDraft, LoanPaymentDraftInput>(`/loans/${id}/payment-draft`, input);
}

export async function postLoanPayment(id: number, input: LoanPaymentDraftInput): Promise<TransactionSummary> {
  return apiPost<TransactionSummary, LoanPaymentDraftInput>(`/loans/${id}/post-payment`, input);
}
