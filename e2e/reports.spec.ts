import { test, expect } from "./fixtures";

// E6 — Reports smoke. Post a couple of transactions, open /reports, run the
// cashflow report, verify at least one row appears with non-zero totals.
test.describe("E6 — Reports smoke", () => {
  test("cashflow report shows posted-transaction movement", async ({ loggedIn: page }) => {
    // Seed an expense account + post two transactions against Cash so the
    // cashflow report has rows to compute.
    await page.evaluate(async () => {
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
        throw new Error(`account create failed: ${acctRes.status}`);
      }
      const accounts = await fetch("/api/v1/accounts?book_id=1", { credentials: "include" }).then(r => r.json());
      const groceries = accounts.find((a: { name: string }) => a.name === "Groceries");
      if (!groceries) throw new Error("Groceries account missing");

      const today = new Date().toISOString().slice(0, 10);
      // Two outflows so the report has data to bucket.
      for (const amount of [4250, 8000]) {
        const res = await fetch("/api/v1/transactions", {
          method: "POST",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            book_id: 1, txn_date: today, payee_id: null, memo: null,
            status: "cleared", reference: null, import_id: null,
            splits: [
              { account_id: 2, commodity_id: 1, amount_minor: -amount, category_id: null, tag_id: null, person_id: null, project_id: null, share_bps: null, memo: null },
              { account_id: groceries.id, commodity_id: 1, amount_minor: amount, category_id: null, tag_id: null, person_id: null, project_id: null, share_bps: null, memo: null },
            ],
          }),
        });
        if (!res.ok) throw new Error(`tx create failed: ${res.status}`);
      }
    });

    await page.goto("/reports");
    await expect(page.getByRole("heading", { name: "Reports" })).toBeVisible();

    // Default tab is "Cash Flow" — cashflow report auto-loads on mount with
    // current-year date range. Wait for the totals hero cards to render.
    await expect(page.getByRole("heading", { name: "Cash Flow Report" })).toBeVisible({
      timeout: 10_000,
    });

    // At least one of the three totals (Inflow / Outflow / Net) shows a
    // non-zero value. We posted $122.50 in outflows.
    await expect(page.getByText("Total Outflow")).toBeVisible();
    // Loose match — "122.50" formatted with locale separators
    await expect(page.locator("body")).toContainText(/122[.,]50/);
  });

  test("category spending tab loads on click", async ({ loggedIn: page }) => {
    // Seed one transaction with a category so spending-by-category has a row.
    await page.evaluate(async () => {
      const catRes = await fetch("/api/v1/categories", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          book_id: 1, parent_id: null, name: "Dining", kind: "expense", color: null,
        }),
      });
      const cat = await catRes.json();

      const acctRes = await fetch("/api/v1/accounts", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          book_id: 1, parent_id: null, account_type: "expense", name: "Food",
          commodity_id: 1, institution_id: null, country_id: null,
          number_last4: null, is_closed: false,
        }),
      });
      const food = await acctRes.json();

      const today = new Date().toISOString().slice(0, 10);
      await fetch("/api/v1/transactions", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          book_id: 1, txn_date: today, payee_id: null, memo: null,
          status: "cleared", reference: null, import_id: null,
          splits: [
            { account_id: 2, commodity_id: 1, amount_minor: -3500, category_id: null, tag_id: null, person_id: null, project_id: null, share_bps: null, memo: null },
            { account_id: food.id, commodity_id: 1, amount_minor: 3500, category_id: cat.id, tag_id: null, person_id: null, project_id: null, share_bps: null, memo: null },
          ],
        }),
      });
    });

    await page.goto("/reports");
    await page.getByRole("tab", { name: "Spending by Category" }).click();

    await expect(page.getByRole("heading", { name: "Spending by Category" })).toBeVisible({
      timeout: 10_000,
    });
    // The seeded "Dining" category appears in the table (the report sums by
    // category name)
    await expect(page.locator("body")).toContainText("Dining");
  });
});
