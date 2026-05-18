import { render, screen, waitFor } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi, beforeEach } from "vitest";
import ResetPasswordPage from "./+page.svelte";
import { requestPasswordReset, confirmPasswordReset } from "$lib/api/auth";
import { page } from "$app/state";

vi.mock("$lib/api/auth", () => ({
  requestPasswordReset: vi.fn(),
  confirmPasswordReset: vi.fn(),
}));

beforeEach(() => {
  (page as unknown as { url: URL }).url = new URL("http://localhost/reset-password");
  vi.mocked(requestPasswordReset).mockResolvedValue();
  vi.mocked(confirmPasswordReset).mockResolvedValue();
});

describe("Reset password page", () => {
  it("requests a reset token without exposing account existence", async () => {
    const user = userEvent.setup();
    render(ResetPasswordPage);

    await user.type(screen.getByLabelText("Email"), "person@example.com");
    await user.click(screen.getByRole("button", { name: "Request Reset" }));

    await waitFor(() => expect(vi.mocked(requestPasswordReset)).toHaveBeenCalledWith("person@example.com"));
    expect(screen.getByText(/If that account exists/)).toBeInTheDocument();
  });

  it("confirms a reset from a token URL", async () => {
    (page as unknown as { url: URL }).url = new URL("http://localhost/reset-password?token=reset-token-fixture");
    const user = userEvent.setup();
    render(ResetPasswordPage);

    await user.type(screen.getByLabelText("New password"), "averystrongpassword");
    await user.type(screen.getByLabelText("Confirm password"), "averystrongpassword");
    await user.click(screen.getByRole("button", { name: "Reset Password" }));

    await waitFor(() => expect(vi.mocked(confirmPasswordReset)).toHaveBeenCalledWith({
      token: "reset-token-fixture",
      new_password: "averystrongpassword",
    }));
  });
});
