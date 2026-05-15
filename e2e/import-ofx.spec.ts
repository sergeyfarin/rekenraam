import { test, expect } from "./fixtures";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const __dirname = dirname(fileURLToPath(import.meta.url));
const OFX_FIXTURE = join(__dirname, "..", "apps", "api", "tests", "data", "sample.ofx");

// E5 — Import an OFX file via the UI; verify rows appear; re-import and
// verify the same rows are flagged as duplicates (gap-plan §1.4.3).
test.describe("E5 — Import OFX", () => {
  test("preview → commit → re-import flags duplicates", async ({ loggedIn: page }) => {
    await page.goto("/import-export");

    // Pick Cash as the target account
    await page.locator("#account").selectOption({ label: "Cash" });
    await page.locator("#format").selectOption("ofx");

    // Upload the canned OFX
    const ofxBuffer = await readFile(OFX_FIXTURE);
    await page.locator('input[type="file"]').setInputFiles({
      name: "sample.ofx",
      mimeType: "application/x-ofx",
      buffer: ofxBuffer,
    });

    // Preview
    await page.getByRole("button", { name: "Preview" }).click();
    // The OFX has 3 transactions → 3 valid rows on first preview.
    await expect(page.getByText(/3 valid, 0 duplicate/)).toBeVisible({ timeout: 10_000 });

    // Commit
    await page.getByRole("button", { name: "Commit" }).click();

    // After commit, the result banner appears
    await expect(page.locator(".bg-muted").filter({ hasText: /Imported|3/ })).toBeVisible({
      timeout: 10_000,
    });

    // Re-import the same file: preview should now show 3 duplicates
    await page.locator('input[type="file"]').setInputFiles({
      name: "sample.ofx",
      mimeType: "application/x-ofx",
      buffer: ofxBuffer,
    });
    await page.getByRole("button", { name: "Preview" }).click();
    await expect(page.getByText(/3 duplicate/)).toBeVisible({ timeout: 10_000 });

    // Walk to the Cash account register and confirm three new rows landed
    await page.goto("/accounts/2");
    // OFX file has WHOLE FOODS MARKET, NETFLIX, ACME PAYROLL — verify they appear
    await expect(page.getByText(/WHOLE FOODS MARKET/i)).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText(/NETFLIX/i)).toBeVisible();
    await expect(page.getByText(/ACME PAYROLL/i)).toBeVisible();
  });
});
