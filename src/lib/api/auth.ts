import { apiGet, apiPost, apiPut } from "./client";

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
    mfa_enabled: boolean;
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

export type LoginResult = {
  mfa_required: boolean;
  challenge_token: string | null;
  user: AuthMe["user"] | null;
  session: AuthMe["session"] | null;
};

export type MfaStatus = {
  enabled: boolean;
  enforced: boolean;
  recovery_codes_remaining: number;
};

export async function getBootstrapStatus(): Promise<BootstrapStatus> {
  return apiGet<BootstrapStatus>("/auth/bootstrap/status");
}

export async function createFirstAdmin(input: BootstrapAdminInput): Promise<AuthMe> {
  return apiPost<AuthMe, BootstrapAdminInput>("/auth/bootstrap/admin", input);
}

export async function login(input: LoginInput): Promise<LoginResult> {
  return apiPost<LoginResult, LoginInput>("/auth/login", input);
}

export async function completeMfaLogin(input: { challenge_token: string; code: string }): Promise<AuthMe> {
  return apiPost<AuthMe, typeof input>("/auth/mfa/login", input);
}

export async function getMfaStatus(): Promise<MfaStatus> {
  return apiGet<MfaStatus>("/auth/mfa/status");
}

export async function beginMfaSetup(current_password: string): Promise<{ setup_key: string; otpauth_uri: string }> {
  return apiPost<{ setup_key: string; otpauth_uri: string }, { current_password: string }>("/auth/mfa/setup", { current_password });
}

export async function confirmMfaSetup(code: string): Promise<{ recovery_codes: string[] }> {
  return apiPost<{ recovery_codes: string[] }, { code: string }>("/auth/mfa/confirm", { code });
}

export async function disableMfa(input: { current_password: string; code: string }): Promise<void> {
  return apiPost<void, typeof input>("/auth/mfa/disable", input);
}

export async function requestPasswordReset(email: string): Promise<void> {
  return apiPost<void, { email: string }>("/auth/password-reset/request", { email });
}

export async function confirmPasswordReset(input: { token: string; new_password: string }): Promise<void> {
  return apiPost<void, typeof input>("/auth/password-reset/confirm", input);
}

export async function acceptInvite(input: { token: string; password: string }): Promise<AuthMe> {
  return apiPost<AuthMe, typeof input>("/auth/invite/accept", input);
}

export async function updateProfile(input: { display_name: string }): Promise<AuthMe> {
  return apiPut<AuthMe, typeof input>("/auth/me/profile", input);
}

export async function changePassword(input: { current_password: string; new_password: string }): Promise<void> {
  return apiPost<void, typeof input>("/auth/me/password", input);
}

export async function logout(): Promise<void> {
  await apiPost<void, Record<string, never>>("/auth/logout", {});
}

export async function getCurrentUser(): Promise<AuthMe> {
  return apiGet<AuthMe>("/auth/me");
}
