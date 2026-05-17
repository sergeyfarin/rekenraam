import { test, expect } from "./fixtures";

// E3 — Reconcile. The seeded Cash account has $5,000.00. Post a small expense,
// open the reconcile page, enter a statement balance that matches the
// post-expense Cash balance, finish without offset. Second pass uses a
// mismatched balance and an offset account.

async function postExpense(page: import("@playwright/test").Page, amountStr: string, payee: string) {
  // Both helpers run inside the page context to reuse the session cookie.
  await page.evaluate(async ({ amountStr, payee }) => {
    // Create the expense account once (idempotent: 409 is fine).
    const acctRes = await fetch("/api/v1/accounts", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        book_id: 1, parent_id: null, account_type: "expense", name: "Groceries",
        commodity_id: 1, institution_id: null, country_id: null,
        number_last4: null, is_closed: false,
      }),
    });
    if (!acctRes.ok && acctRes.status !== 409) {
      throw new Error(`account create failed: ${acctRes.status} ${await acctRes.text()}`);
    }
    const accounts = await fetch("/api/v1/accounts?book_id=1", { credentials: "include" }).then(r => r.json());
    const groceries = accounts.find((a: { name: string }) => a.name === "Groceries");
    if (!groceries) throw new Error("Groceries account missing after create");

    const payeeRes = await fetch("/api/v1/payees", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ book_id: 1, name: payee, kind: "person", metadata: null }),
    });
    const payeeRow = await payeeRes.json();

    // amountStr is positive; the Cash leg is negative.
    const amount_minor = Math.round(Number(amountStr) * 100);
    const today = new Date().toISOString().slice(0, 10);
    const txRes = await fetch("/api/v1/transactions", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        book_id: 1,
        txn_date: today,
        payee_id: payeeRow.id,
        memo: null,
        status: "uncleared",
        reference: null,
        import_id: null,
        splits: [
          { account_id: 2, commodity_id: 1, amount_minor: -amount_minor, category_id: null, tag_id: null, person_id: null, project_id: null, share_bps: null, memo: null },
          { account_id: groceries.id, commodity_id: 1, amount_minor, category_id: null, tag_id: null, person_id: null, project_id: null, share_bps: null, memo: null },
        ],
      }),
    });
    if (!txRes.ok) throw new Error(`tx create failed: ${txRes.status} ${await txRes.text()}`);
  }, { amountStr, payee });
}

test.describe("E3 — Reconcile", () => {
  test("matched balance finishes without an offset", async ({ loggedIn: page }) => {
    // Cash opens at $5,000.00. Post $42.50 expense → cleared cash = $4,957.50
    await postExpense(page, "42.50", "Test Grocer 1");

    await page.goto("/accounts/2/reconcile");
    await expect(page.getByText("Reconcile: Cash")).toBeVisible();

    // Statement end date defaults to today; balance is the post-expense figure.
    await page.getByLabel("Statement ending balance").fill("4957.50");
    await page.getByRole("button", { name: "Start" }).click();

    // Working step: shows Opening / Selected / Cleared / Difference
    await expect(page.getByText("Opening", { exact: true }).first()).toBeVisible();

    // The seeded opening transaction is preselected; select the freshly-posted
    // expense too so cleared balance matches the statement.
    await page
      .locator("label")
      .filter({ hasText: "Test Grocer 1" })
      .getByLabel("Mark as reconciled")
      .check();

    // Balanced: no offset dropdown
    await expect(page.getByLabel("Offset account")).not.toBeVisible();

    await page.getByRole("button", { name: "Finish reconciliation" }).click();

    // On success we navigate back to /accounts/2
    await expect(page).toHaveURL(/\/accounts\/2$/);
  });

  test("mismatched balance requires an offset account", async ({ loggedIn: page }) => {
    await postExpense(page, "100.00", "Test Grocer 2");
    // Cash should now be $4,900.00. Pretend statement says $4,910.00 → $10 short.

    // Need a peer asset account to act as the offset. Create one.
    await page.evaluate(async () => {
      const res = await fetch("/api/v1/accounts", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          book_id: 1, parent_id: null, account_type: "asset", name: "Offset Account",
          commodity_id: 1, institution_id: null, country_id: null,
          number_last4: null, is_closed: false,
        }),
      });
      if (!res.ok) throw new Error(`offset account create failed: ${res.status}`);
    });

    await page.goto("/accounts/2/reconcile");
    await page.getByLabel("Statement ending balance").fill("4910.00");
    await page.getByRole("button", { name: "Start" }).click();
    await expect(page.getByText("Opening", { exact: true }).first()).toBeVisible();

    // Don't check anything: opening balance ($5,000) is preselected.
    // Statement says $4,910 → diff = -$90 → offset required.
    await expect(page.getByLabel("Offset account", { exact: true })).toBeVisible();

    // Finish disabled until offset selected
    const finishBtn = page.getByRole("button", { name: "Finish reconciliation" });
    await expect(finishBtn).toBeDisabled();

    await page.getByLabel("Offset account", { exact: true }).selectOption({ label: "Offset Account" });
    await expect(finishBtn).toBeEnabled();
    await finishBtn.click();

    await expect(page).toHaveURL(/\/accounts\/2$/);
  });
});
