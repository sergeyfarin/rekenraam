import { render, screen, waitFor } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AcceptInvitePage from "./+page.svelte";
import { acceptInvite } from "$lib/api/auth";
import { initializeBookContext } from "$lib/book-context";
import { page } from "$app/state";
import { goto } from "$app/navigation";

vi.mock("$lib/api/auth", () => ({
  acceptInvite: vi.fn(),
}));

vi.mock("$lib/book-context", () => ({
  initializeBookContext: vi.fn(),
}));

beforeEach(() => {
  (page as unknown as { url: URL }).url = new URL("http://localhost/accept-invite?token=invite-token-123456");
  vi.mocked(acceptInvite).mockResolvedValue({
    user: { id: 2, email: "new@example.com", display_name: "New User", is_admin: false, is_active: true, mfa_enabled: false },
    session: { id: 10, device_id: null, expires_at: "2026-06-01T00:00:00Z" },
  });
  vi.mocked(initializeBookContext).mockResolvedValue({
    books: [],
    activeBook: null,
    loading: false,
    error: null,
  });
  vi.mocked(goto).mockClear();
});

describe("Accept invite page", () => {
  it("accepts an invite and redirects home", async () => {
    const user = userEvent.setup();
    render(AcceptInvitePage);

    await user.type(screen.getByLabelText("Password"), "averystrongpassword");
    await user.type(screen.getByLabelText("Confirm password"), "averystrongpassword");
    await user.click(screen.getByRole("button", { name: "Accept Invite" }));

    await waitFor(() => expect(vi.mocked(acceptInvite)).toHaveBeenCalledWith({
      token: "invite-token-123456",
      password: "averystrongpassword",
    }));
    expect(vi.mocked(goto)).toHaveBeenCalledWith("/");
  });
});
