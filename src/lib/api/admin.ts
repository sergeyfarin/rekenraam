import { apiGet, apiPost } from "$lib/api/client";

export type AdminRuntimeStatus = {
  database_kind: string;
  database_name: string;
  database_host: string | null;
  display_path: string;
  size_bytes: number | null;
  writable: boolean;
  foreign_keys: boolean;
  current_version: string | null;
  latest_version: string;
  pending_versions: string[];
  note: string;
};

export type FiscalYearCloseResponse = {
  tx_id: number | null;
  closed_accounts_count: number;
  retained_earnings_delta_minor: number;
  close_date: string;
};

export async function closeFiscalYear(input: { close_date: string; memo: string | null }): Promise<FiscalYearCloseResponse> {
  return apiPost<FiscalYearCloseResponse, typeof input>("/admin/fiscal-year-close", input);
}

export async function getAdminRuntimeStatus(): Promise<AdminRuntimeStatus> {
  return apiGet<AdminRuntimeStatus>("/admin/runtime");
}

export async function dbIntegrityCheck(): Promise<string> {
  const response = await apiPost<{ status: string }, Record<string, never>>("/admin/integrity-check", {});
  return response.status;
}