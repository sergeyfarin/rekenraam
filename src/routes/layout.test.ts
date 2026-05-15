import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/svelte";

vi.mock("$lib/api/auth", () => ({
  completeMfaLogin: vi.fn(),
  createFirstAdmin: vi.fn(),
  getBootstrapStatus: vi.fn(),
  getCurrentUser: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
}));

import {
  completeMfaLogin,
  createFirstAdmin,
  getBootstrapStatus,
  getCurrentUser,
  login,
  logout,
} from "$lib/api/auth";
import Layout from "./+layout.svelte";

function authedUser() {
  return {
    user: {
      id: 1,
      email: "user@example.com",
      display_name: "Test User",
      is_admin: true,
      is_active: true,
      mfa_enabled: false,
    },
    session: {
      id: 1,
      device_id: null,
      expires_at: "2099-01-01T00:00:00Z",
    },
  };
}

beforeEach(() => {
  vi.resetAllMocks();
});

afterEach(() => {
  vi.resetAllMocks();
});

// ─── C1: Bootstrap flow ────────────────────────────────────────────────────

describe("Layout — bootstrap flow (C1)", () => {
  it("shows Create Admin form when bootstrap_required is true", async () => {
    vi.mocked(getCurrentUser).mockRejectedValue(new Error("unauthenticated"));
    vi.mocked(getBootstrapStatus).mockResolvedValue({ bootstrap_required: true });

    render(Layout);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Create Admin" })).toBeInTheDocument();
    });
    expect(screen.getByPlaceholderText("Display name")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Email")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Password")).toBeInTheDocument();
  });

  it("calls createFirstAdmin with form values on submit", async () => {
    vi.mocked(getCurrentUser).mockRejectedValue(new Error("unauthenticated"));
    vi.mocked(getBootstrapStatus).mockResolvedValue({ bootstrap_required: true });
    vi.mocked(createFirstAdmin).mockResolvedValue(authedUser());

    render(Layout);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Create Admin" })).toBeInTheDocument();
    });

    await fireEvent.input(screen.getByPlaceholderText("Display name"), {
      target: { value: "Admin User" },
    });
    await fireEvent.input(screen.getByPlaceholderText("Email"), {
      target: { value: "admin@example.com" },
    });
    await fireEvent.input(screen.getByPlaceholderText("Password"), {
      target: { value: "supersecretpw1234" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Create Admin" }));

    await waitFor(() => expect(vi.mocked(createFirstAdmin)).toHaveBeenCalledOnce());
    expect(vi.mocked(createFirstAdmin).mock.calls[0][0]).toEqual({
      email: "admin@example.com",
      password: "supersecretpw1234",
      display_name: "Admin User",
    });
  });
});

// ─── C1: Login + MFA flow ──────────────────────────────────────────────────

describe("Layout — login flow (C1)", () => {
  it("shows Sign In form when not authed and bootstrap is complete", async () => {
    vi.mocked(getCurrentUser).mockRejectedValue(new Error("unauthenticated"));
    vi.mocked(getBootstrapStatus).mockResolvedValue({ bootstrap_required: false });

    render(Layout);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Sign In" })).toBeInTheDocument();
    });
    // No display-name field in plain sign-in
    expect(screen.queryByPlaceholderText("Display name")).not.toBeInTheDocument();
  });

  it("login without MFA renders the navigation chrome", async () => {
    vi.mocked(getCurrentUser).mockRejectedValue(new Error("unauthenticated"));
    vi.mocked(getBootstrapStatus).mockResolvedValue({ bootstrap_required: false });
    vi.mocked(login).mockResolvedValue({
      mfa_required: false,
      challenge_token: null,
      user: authedUser().user,
      session: authedUser().session,
    });

    render(Layout);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Sign In" })).toBeInTheDocument();
    });
    await fireEvent.input(screen.getByPlaceholderText("Email"), {
      target: { value: "user@example.com" },
    });
    await fireEvent.input(screen.getByPlaceholderText("Password"), {
      target: { value: "passw0rd" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Sign In" }));

    await waitFor(() => expect(vi.mocked(login)).toHaveBeenCalledOnce());
    // Once authed, the nav appears
    await waitFor(() => {
      expect(screen.getByRole("navigation", { name: "Primary" })).toBeInTheDocument();
    });
  });

  it("MFA-required login swaps to TOTP form; submitting calls completeMfaLogin", async () => {
    vi.mocked(getCurrentUser).mockRejectedValue(new Error("unauthenticated"));
    vi.mocked(getBootstrapStatus).mockResolvedValue({ bootstrap_required: false });
    vi.mocked(login).mockResolvedValue({
      mfa_required: true,
      challenge_token: "tok-abc",
      user: null,
      session: null,
    });
    vi.mocked(completeMfaLogin).mockResolvedValue(authedUser());

    render(Layout);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Sign In" })).toBeInTheDocument();
    });
    await fireEvent.input(screen.getByPlaceholderText("Email"), {
      target: { value: "user@example.com" },
    });
    await fireEvent.input(screen.getByPlaceholderText("Password"), {
      target: { value: "passw0rd" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Sign In" }));

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Two-factor code" })).toBeInTheDocument();
    });
    const totpInput = screen.getByPlaceholderText(/Authenticator or recovery code/);
    await fireEvent.input(totpInput, { target: { value: "123456" } });
    await fireEvent.click(screen.getByRole("button", { name: "Verify" }));

    await waitFor(() => expect(vi.mocked(completeMfaLogin)).toHaveBeenCalledOnce());
    expect(vi.mocked(completeMfaLogin).mock.calls[0][0]).toEqual({
      challenge_token: "tok-abc",
      code: "123456",
    });
  });

  it("surfaces login error message when login() throws", async () => {
    vi.mocked(getCurrentUser).mockRejectedValue(new Error("unauthenticated"));
    vi.mocked(getBootstrapStatus).mockResolvedValue({ bootstrap_required: false });
    vi.mocked(login).mockRejectedValue(new Error("Invalid credentials"));

    render(Layout);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Sign In" })).toBeInTheDocument();
    });
    await fireEvent.input(screen.getByPlaceholderText("Email"), {
      target: { value: "user@example.com" },
    });
    await fireEvent.input(screen.getByPlaceholderText("Password"), {
      target: { value: "wrong" },
    });
    await fireEvent.click(screen.getByRole("button", { name: "Sign In" }));

    expect(await screen.findByText("Invalid credentials")).toBeInTheDocument();
  });
});

// ─── C6: Session-expired ───────────────────────────────────────────────────

describe("Layout — session-expired (C6)", () => {
  it("shows Sign In form (not the nav) when getCurrentUser rejects (401-like)", async () => {
    vi.mocked(getCurrentUser).mockRejectedValue(new Error("401 Unauthorized"));
    vi.mocked(getBootstrapStatus).mockResolvedValue({ bootstrap_required: false });

    render(Layout);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Sign In" })).toBeInTheDocument();
    });
    expect(screen.queryByRole("navigation", { name: "Primary" })).not.toBeInTheDocument();
  });

  it("Logout calls logout() and reloads back to Sign In", async () => {
    vi.mocked(getCurrentUser)
      .mockResolvedValueOnce(authedUser())
      .mockRejectedValue(new Error("unauthenticated"));
    vi.mocked(getBootstrapStatus).mockResolvedValue({ bootstrap_required: false });
    vi.mocked(logout).mockResolvedValue();

    render(Layout);
    await waitFor(() => {
      expect(screen.getByRole("navigation", { name: "Primary" })).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole("button", { name: "Logout" }));

    await waitFor(() => expect(vi.mocked(logout)).toHaveBeenCalledOnce());
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Sign In" })).toBeInTheDocument();
    });
  });

  it("renders the auth-loading state before getCurrentUser resolves", async () => {
    let resolveCurrentUser: (value: never) => void = () => {};
    vi.mocked(getCurrentUser).mockReturnValue(
      new Promise((_, reject) => {
        resolveCurrentUser = () => reject(new Error("unauthenticated")) as never;
      }),
    );
    vi.mocked(getBootstrapStatus).mockResolvedValue({ bootstrap_required: false });

    render(Layout);
    expect(screen.getByText("Loading")).toBeInTheDocument();
    resolveCurrentUser(undefined as never);
    await waitFor(() => {
      expect(screen.queryByText("Loading")).not.toBeInTheDocument();
    });
  });
});
