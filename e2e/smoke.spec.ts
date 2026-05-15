import { test, expect, ADMIN_DISPLAY_NAME } from "./fixtures";

test.describe("Smoke (A2 acceptance)", () => {
  test("loads the login form on an unbootstrapped stack", async ({ page }) => {
    await page.goto("/");
    // Fresh DB → bootstrap_required: true → Create Admin form
    await expect(page.getByRole("heading", { name: "Create Admin" })).toBeVisible();
  });

  test("loggedIn fixture lands on the authenticated home view", async ({ loggedIn }) => {
    // The fixture has already bootstrapped + navigated to "/". Assert the
    // nav chrome rendered, indicating successful auth.
    await expect(loggedIn.getByRole("navigation", { name: "Primary" })).toBeVisible();
    await expect(loggedIn.getByText(ADMIN_DISPLAY_NAME)).toBeVisible();
  });
});
