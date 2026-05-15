import { test, expect } from "./fixtures";

// E2 — Post a transaction via the UI and verify it appears in the
// transactions list AND updates the account balance shown on /accounts/{id}.
//
// The seeded `personal` book ships with:
//   • account_id=2  "Cash" (asset)
//   • account_id=3  "Opening Balances" (equity, is_system)
//   • a pre-posted $5,000.00 opening balance into Cash
// (see apps/api/alembic/versions/0001_initial_schema.py)
test.describe("E2 — Post a transaction", () => {
  test("create → list → balance reflects", async ({ loggedIn: page }) => {
    // Need an expense category and a peer account to transfer to. The seed
    // doesn't include those; create them via the lightweight metadata APIs
    // we already exercise from the frontend.
    await page.evaluate(async () => {
      const res = await fetch("/api/v1/accounts", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          book_id: 1,
          parent_id: null,
          account_type: "expense",
          name: "Groceries",
          commodity_id: 1,
          institution_id: null,
          country_id: null,
          number_last4: null,
          is_closed: false,
        }),
      });
      if (!res.ok) throw new Error(`account create failed: ${res.status} ${await res.text()}`);
    });

    // Open the transactions page and click New
    await page.goto("/transactions");
    await page.getByRole("button", { name: "New transaction" }).click();
    await expect(page.getByRole("heading", { name: "New transaction" })).toBeVisible();

    // Date defaults to today; leave as-is. Pick Cash as the account.
    const accountSelect = page.locator("#tx-account");
    await accountSelect.selectOption({ label: "Cash" });

    // Pick the Groceries expense account as the transfer account
    const transferSelect = page.locator("#tx-transfer-account");
    await transferSelect.selectOption({ label: "Groceries" });

    // Fill payee, amount; leave memo + reference empty
    await page.locator("#tx-payee").fill("Test Grocer");
    await page.locator("#tx-amount").fill("-42.50");

    await page.getByRole("button", { name: "Save" }).click();

    // The "create payee" confirm dialog opens (the payee doesn't exist yet)
    await expect(page.getByText(/Create new payee "Test Grocer"/)).toBeVisible();
    await page.getByRole("button", { name: "Create" }).click();

    // After save, the dialog closes and the row appears in the list
    await expect(page.getByRole("heading", { name: "New transaction" })).not.toBeVisible();
    await expect(page.getByRole("cell", { name: "Test Grocer", exact: false })).toBeVisible({
      timeout: 5_000,
    });

    // The Cash account balance was $5000.00; -$42.50 should land it at $4957.50.
    // Navigate to the Cash account page (id=2) and verify the balance row.
    await page.goto("/accounts/2");
    // Header card shows e.g. "Balance: 4957.50 USD"
    await expect(page.getByText(/Balance:.*4957\.50/)).toBeVisible({ timeout: 5_000 });

    // The new transaction appears in the register
    await expect(page.locator(".data-table")).toContainText("Test Grocer");
  });
});
