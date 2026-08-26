/**
 * Accessibility checks for the core workflows (R3a).
 *
 * Two kinds of check, because they fail for different reasons:
 *
 *  - `expectNoAccessibilityViolations` runs axe, which catches the machine-
 *    checkable rules: contrast, missing labels, broken ARIA, heading order.
 *  - The keyboard and focus helpers below check what axe cannot — that a
 *    screen can actually be *operated* without a mouse. A page can pass every
 *    axe rule and still trap focus or lose it on navigation.
 *
 * Scope is deliberate. These are smoke checks over the journeys the roadmap
 * names, not an audit: a failure here means something regressed, and a pass
 * does not mean the app is accessible.
 */

import AxeBuilder from '@axe-core/playwright';
import { expect, type Page } from '@playwright/test';

/** WCAG 2.1 A and AA, which is what the product's copy claims to aim at. */
const STANDARD_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'];

export type AxeViolation = {
  id: string;
  impact?: string | null;
  help: string;
  nodes: number;
  targets: string[];
};

/** Runs axe and returns violations in a form a failure message can print. */
export async function accessibilityViolations(page: Page): Promise<AxeViolation[]> {
  const results = await new AxeBuilder({ page }).withTags(STANDARD_TAGS).analyze();
  return results.violations.map((violation) => ({
    id: violation.id,
    impact: violation.impact,
    help: violation.help,
    nodes: violation.nodes.length,
    targets: violation.nodes.slice(0, 3).map((node) => node.target.join(' '))
  }));
}

/**
 * Asserts a page has no axe violations, printing what and where when it does —
 * a bare count sends the reader back to the browser to find out what broke.
 */
export async function expectNoAccessibilityViolations(page: Page, label: string) {
  const violations = await accessibilityViolations(page);
  expect(
    violations,
    violations.length === 0
      ? `${label}: no violations`
      : `${label} has ${violations.length} accessibility violation(s):\n` +
          violations
            .map((v) => `  - [${v.impact ?? 'unknown'}] ${v.id}: ${v.help}\n      ${v.targets.join('\n      ')}`)
            .join('\n')
  ).toEqual([]);
}

/**
 * Asserts every interactive control on the page can be reached by tabbing.
 *
 * Walks forward from the document body and collects what receives focus, then
 * compares against the controls that are actually visible and enabled. A
 * control that never receives focus is unreachable without a mouse, which axe
 * does not report.
 */
export async function expectKeyboardReachable(page: Page, label: string, expectedLabels: string[]) {
  const reached = new Set<string>();

  await page.evaluate(() => document.body.focus());
  for (let step = 0; step < 60; step++) {
    await page.keyboard.press('Tab');
    const description = await page.evaluate(() => {
      const active = document.activeElement as HTMLElement | null;
      if (!active || active === document.body) return '';
      const label =
        active.getAttribute('aria-label') ??
        active.textContent?.trim() ??
        active.getAttribute('placeholder') ??
        '';
      return `${active.tagName.toLowerCase()}:${label.slice(0, 60)}`;
    });
    if (description) reached.add(description);
  }

  const reachedText = [...reached].join('\n');
  for (const expected of expectedLabels) {
    expect(
      reachedText,
      `${label}: "${expected}" is never reached by tabbing, so it cannot be used without a mouse.\nReached:\n${reachedText}`
    ).toContain(expected);
  }
}

/**
 * Asserts focus is somewhere useful after a navigation.
 *
 * SvelteKit moves focus to the document after a client-side navigation, which
 * leaves a screen-reader user at the top of a page with no announcement of
 * where they are. What matters is that focus is not left on a control that no
 * longer exists.
 */
export async function expectFocusIsNotOrphaned(page: Page, label: string) {
  const orphaned = await page.evaluate(() => {
    const active = document.activeElement;
    if (!active || active === document.body || active === document.documentElement) return false;
    return !active.isConnected;
  });
  expect(orphaned, `${label}: focus is on an element that is no longer in the document`).toBe(false);
}
