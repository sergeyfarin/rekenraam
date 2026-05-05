import { apiDelete, apiGet, apiPost, apiPut } from "$lib/api/client";

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

export type BookMembership = {
  book_id: number;
  book_slug: string;
  book_name: string;
  role: "owner" | "editor" | "viewer";
};

export type AdminUser = {
  id: number;
  email: string;
  display_name: string;
  is_admin: boolean;
  is_active: boolean;
  created_at: string;
  updated_at: string;
  memberships: BookMembership[];
};

export type AuditEvent = {
  id: number;
  book_id: number | null;
  actor_user_id: number | null;
  event_type: string;
  target_type: string | null;
  target_id: number | null;
  summary: string;
  metadata: Record<string, unknown> | null;
  created_at: string;
};

export async function listAdminUsers(): Promise<AdminUser[]> {
  return apiGet<AdminUser[]>("/admin/users");
}

export async function createAdminUser(input: {
  email: string;
  display_name: string;
  password: string;
  is_admin: boolean;
  memberships: [number, "owner" | "editor" | "viewer"][];
}): Promise<AdminUser> {
  return apiPost<AdminUser, typeof input>("/admin/users", input);
}

export async function updateAdminUser(userId: number, input: {
  display_name?: string;
  is_admin?: boolean;
  is_active?: boolean;
}): Promise<AdminUser> {
  return apiPut<AdminUser, typeof input>(`/admin/users/${userId}`, input);
}

export async function resetAdminUserPassword(userId: number, password: string): Promise<AdminUser> {
  return apiPost<AdminUser, { password: string }>(`/admin/users/${userId}/password`, { password });
}

export async function revokeAdminUserSessions(userId: number): Promise<number> {
  return apiPost<number, Record<string, never>>(`/admin/users/${userId}/sessions/revoke`, {});
}

export async function setAdminUserMembership(userId: number, input: {
  book_id: number;
  role: "owner" | "editor" | "viewer";
}): Promise<AdminUser> {
  return apiPut<AdminUser, typeof input>(`/admin/users/${userId}/memberships`, input);
}

export async function deleteAdminUserMembership(userId: number, bookId: number): Promise<AdminUser> {
  return apiDelete<AdminUser>(`/admin/users/${userId}/memberships/${bookId}`);
}

export async function listAuditEvents(input: { book_id?: number; limit?: number } = {}): Promise<AuditEvent[]> {
  const params = new URLSearchParams();
  if (input.book_id !== undefined) params.set("book_id", String(input.book_id));
  if (input.limit !== undefined) params.set("limit", String(input.limit));
  const query = params.toString();
  return apiGet<AuditEvent[]>(`/admin/audit-events${query ? `?${query}` : ""}`);
}
