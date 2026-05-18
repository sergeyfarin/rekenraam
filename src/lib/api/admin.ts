import { apiDelete, apiGet, apiPost, apiPut } from "$lib/api/client";

export type AdminRuntimeStatus = {
  database_kind: string;
  database_name: string;
  database_host: string | null;
  database_user: string | null;
  database_version: string | null;
  display_path: string;
  size_bytes: number | null;
  writable: boolean;
  foreign_keys: boolean;
  current_version: string | null;
  latest_version: string;
  pending_versions: string[];
  pending_migration_count: number;
  health_status: string;
  backup_status: string;
  backup_guidance: string;
  last_integrity_status: string | null;
  last_integrity_checked_at: string | null;
  note: string;
};

export type IntegrityCheck = {
  status: string;
  checked_at: string | null;
  checks: { name: string; status: string; detail: string }[];
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

export async function dbIntegrityCheck(): Promise<IntegrityCheck> {
  return apiPost<IntegrityCheck, Record<string, never>>("/admin/integrity-check", {});
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
  mfa_required: boolean;
  created_at: string;
  updated_at: string;
  memberships: BookMembership[];
};

export type AdminInviteCreated = {
  user: AdminUser;
  token: string;
  expires_at: string;
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

export async function createAdminInvite(input: {
  email: string;
  display_name: string;
  is_admin: boolean;
  memberships: [number, "owner" | "editor" | "viewer"][];
}): Promise<AdminInviteCreated> {
  return apiPost<AdminInviteCreated, typeof input>("/admin/invites", input);
}

export async function updateAdminUser(userId: number, input: {
  display_name?: string;
  is_admin?: boolean;
  is_active?: boolean;
  mfa_required?: boolean;
}): Promise<AdminUser> {
  return apiPut<AdminUser, typeof input>(`/admin/users/${userId}`, input);
}

export async function resetAdminUserMfa(userId: number): Promise<void> {
  return apiDelete<void>(`/admin/users/${userId}/mfa`);
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
