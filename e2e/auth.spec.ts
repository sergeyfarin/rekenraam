import { test, expect, ADMIN_EMAIL, ADMIN_PASSWORD, ADMIN_DISPLAY_NAME } from "./fixtures";

// E1 — Auth round-trip. Bootstrap admin via the UI, log out, log back in,
// then prove the protected page is gated when the session cookie is cleared.
test.describe("E1 — Auth round-trip", () => {
  test("bootstrap → logout → re-login → session-clear gates protected pages", async ({ page, context }) => {
    // Fresh DB (auto-fixture); page lands on Create Admin form.
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Create Admin" })).toBeVisible();

    // Bootstrap the admin
    await page.getByPlaceholder("Display name").fill(ADMIN_DISPLAY_NAME);
    await page.getByPlaceholder("Email").fill(ADMIN_EMAIL);
    await page.getByPlaceholder("Password").fill(ADMIN_PASSWORD);
    await page.getByRole("button", { name: "Create Admin" }).click();

    // Bootstrap auto-authenticates → nav chrome visible
    await expect(page.getByRole("navigation", { name: "Primary" })).toBeVisible();
    await expect(page.getByText(ADMIN_DISPLAY_NAME)).toBeVisible();

    // Logout
    await page.getByRole("button", { name: "Logout" }).click();
    await expect(page.getByRole("heading", { name: "Sign In" })).toBeVisible();

    // Re-login with the same creds
    await page.getByPlaceholder("Email").fill(ADMIN_EMAIL);
    await page.getByPlaceholder("Password").fill(ADMIN_PASSWORD);
    await page.getByRole("button", { name: "Sign In" }).click();
    await expect(page.getByRole("navigation", { name: "Primary" })).toBeVisible();

    // Clear cookies to simulate session expiry, then navigate to a protected page.
    // Expectation: the layout falls back to the Sign In form (no nav chrome).
    await context.clearCookies();
    await page.goto("/transactions");
    await expect(page.getByRole("heading", { name: "Sign In" })).toBeVisible();
    await expect(page.getByRole("navigation", { name: "Primary" })).not.toBeVisible();
  });

  test("wrong password surfaces error and stays on Sign In", async ({ page, authedApi }) => {
    // authedApi fixture bootstraps the admin so we have someone to fail against.
    // Force-logout the browser context: we never authenticated this page, so
    // it should render Sign In.
    void authedApi;
    await page.goto("/");
    await expect(page.getByRole("heading", { name: "Sign In" })).toBeVisible();

    await page.getByPlaceholder("Email").fill(ADMIN_EMAIL);
    await page.getByPlaceholder("Password").fill("not-the-real-password");
    await page.getByRole("button", { name: "Sign In" }).click();

    // Error message renders (exact text comes from API "Invalid email or password" or similar)
    await expect(page.locator("p.text-destructive")).toBeVisible();
    await expect(page.getByRole("heading", { name: "Sign In" })).toBeVisible();
  });
});
