import { apiGet, apiPost } from "./client";

export type BootstrapStatus = {
  bootstrap_required: boolean;
};

export type AuthMe = {
  user: {
    id: number;
    email: string;
    display_name: string;
    is_admin: boolean;
    is_active: boolean;
  };
  session: {
    id: number;
    device_id: number | null;
    expires_at: string;
  };
};

export type BootstrapAdminInput = {
  email: string;
  password: string;
  display_name: string;
};

export type LoginInput = {
  email: string;
  password: string;
};

export async function getBootstrapStatus(): Promise<BootstrapStatus> {
  return apiGet<BootstrapStatus>("/auth/bootstrap/status");
}

export async function createFirstAdmin(input: BootstrapAdminInput): Promise<AuthMe> {
  return apiPost<AuthMe, BootstrapAdminInput>("/auth/bootstrap/admin", input);
}

export async function login(input: LoginInput): Promise<AuthMe> {
  return apiPost<AuthMe, LoginInput>("/auth/login", input);
}

export async function logout(): Promise<void> {
  await apiPost<void, Record<string, never>>("/auth/logout", {});
}

export async function getCurrentUser(): Promise<AuthMe> {
  return apiGet<AuthMe>("/auth/me");
}
