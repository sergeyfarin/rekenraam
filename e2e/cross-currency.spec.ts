import { test, expect } from "./fixtures";

// E4 — Cross-currency transfer. The seeded book ships with one currency
// (USD, commodity_id=1). To exercise the cross-currency code path we need a
// second commodity (EUR) and an account that holds it. Both are created via
// the public API before driving the UI flow.

test.describe("E4 — Cross-currency transfer", () => {
  test("posts a USD → EUR transfer via the UI and reflects both legs", async ({ loggedIn: page }) => {
    // Seed a EUR commodity + EUR account ("EUR Wallet"). Commodities are
    // created via the /currencies endpoint (the public metadata module
    // exposes currency CRUD, not raw commodity CRUD).
    const { eurAccountId } = await page.evaluate(async () => {
      const eurCommodity = await fetch("/api/v1/currencies", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          book_id: 1, symbol: "EUR", display_symbol: "€", name: "Euro", scale: 2,
        }),
      }).then(async (r) => {
        if (!r.ok) throw new Error(`currency create failed: ${r.status} ${await r.text()}`);
        return r.json();
      });

      const eurAccount = await fetch("/api/v1/accounts", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          book_id: 1, parent_id: null, account_type: "asset", name: "EUR Wallet",
          commodity_id: eurCommodity.id, institution_id: null, country_id: null,
          number_last4: null, is_closed: false,
        }),
      }).then(async (r) => {
        if (!r.ok) throw new Error(`account create failed: ${r.status} ${await r.text()}`);
        return r.json();
      });

      return { eurAccountId: eurAccount.id as number };
    });

    await page.goto("/transactions");
    await page.getByRole("button", { name: "Cross-currency transfer…" }).click();
    await expect(page.getByRole("heading", { name: "Cross-currency transfer" })).toBeVisible();

    // Source = Cash (USD, seeded $5000, account_id=2); Destination = EUR Wallet
    // The seeded USD has symbol "USD", so the option label is e.g. "Cash (USD)".
    await page.locator("#xfer-source").selectOption({ value: "2" });
    await page.locator("#xfer-destination").selectOption({ value: String(eurAccountId) });

    // Transfer $100 → €90 at rate 0.90
    await page.locator("#xfer-source-amount").fill("100.00");
    await page.locator("#xfer-destination-amount").fill("90.00");
    await page.locator("#xfer-rate").fill("0.90");

    await page.getByRole("button", { name: "Post transfer" }).click();

    // Dialog closes on success
    await expect(page.getByRole("heading", { name: "Cross-currency transfer" })).not.toBeVisible({
      timeout: 5_000,
    });

    // Verify Cash balance dropped by $100 → $4,900.00
    await page.goto("/accounts/2");
    await expect(page.getByText(/Balance:.*4900\.00/)).toBeVisible({ timeout: 5_000 });

    // Verify EUR Wallet balance is €90.00
    await page.goto(`/accounts/${eurAccountId}`);
    await expect(page.getByText(/Balance:.*90\.00/)).toBeVisible({ timeout: 5_000 });
  });

  test("validation: identical source and destination is rejected", async ({ loggedIn: page }) => {
    // Add a second USD account so the dropdown has two entries
    await page.evaluate(async () => {
      await fetch("/api/v1/accounts", {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          book_id: 1, parent_id: null, account_type: "asset", name: "Spare USD",
          commodity_id: 1, institution_id: null, country_id: null,
          number_last4: null, is_closed: false,
        }),
      });
    });

    await page.goto("/transactions");
    await page.getByRole("button", { name: "Cross-currency transfer…" }).click();

    await page.locator("#xfer-source").selectOption({ value: "2" });
    await page.locator("#xfer-destination").selectOption({ value: "2" });
    await page.locator("#xfer-source-amount").fill("10.00");
    await page.locator("#xfer-destination-amount").fill("10.00");
    await page.locator("#xfer-rate").fill("1.0");
    await page.getByRole("button", { name: "Post transfer" }).click();

    await expect(page.getByText(/Source and destination accounts must differ/)).toBeVisible();
  });
});
