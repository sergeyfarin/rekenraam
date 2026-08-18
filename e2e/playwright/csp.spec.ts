/**
 * The app must run clean under the `Content-Security-Policy` the Go middleware
 * sends (`withSecurityHeaders` in `backend/internal/api/middleware.go`).
 *
 * A violation on every page load is not harmless: it buries the one that
 * matters. T-46 was exactly that — SvelteKit's `#svelte-announcer` carries its
 * visually-hidden rules in a `style` attribute, which `style-src 'self'`
 * refuses through `style-src-attr` when Svelte parses the compiled template.
 * Nothing looked broken, so it went unnoticed for as long as it took someone to
 * read the console.
 */

import { expect, test } from '@playwright/test';
import { ensureBrowserSession } from './support/session';

type Violation = {
  directive: string;
  blockedURI: string;
  sample: string;
  sourceFile: string;
  line: number;
};

test('runs without Content-Security-Policy violations', async ({ page }) => {
  // Must be an init script: the announcer's template is parsed during bundle
  // startup, well before any test code could attach a listener.
  await page.addInitScript(() => {
    const violations: Violation[] = ((window as never as { __cspViolations: Violation[] }).__cspViolations = []);
    document.addEventListener('securitypolicyviolation', (event) => {
      violations.push({
        directive: event.effectiveDirective,
        blockedURI: event.blockedURI,
        sample: event.sample,
        sourceFile: event.sourceFile,
        line: event.lineNumber
      });
    });
  });

  await ensureBrowserSession(page);

  for (const route of ['/app/transactions', '/app/accounts', '/app/reports', '/app/settings']) {
    await page.goto(route);
    await expect(page.locator('main')).toBeVisible();
  }

  // The announcer only exists after the client-side router has mounted, and it
  // is the element T-46 was about — assert it is still hidden without its
  // inline style attribute rather than merely absent.
  const announcer = page.locator('#svelte-announcer');
  await expect(announcer).toHaveCount(1);
  expect(await announcer.getAttribute('style')).toBeNull();
  const box = await announcer.boundingBox();
  expect(box?.width).toBeLessThanOrEqual(1);
  expect(box?.height).toBeLessThanOrEqual(1);

  const violations = await page.evaluate(() => (window as never as { __cspViolations: Violation[] }).__cspViolations);
  expect(violations, `CSP violations: ${JSON.stringify(violations, null, 2)}`).toEqual([]);
});
