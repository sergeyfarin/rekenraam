import { expect, test, type Page } from './support/fixtures';
import { accessibilityViolations } from './support/a11y';
import { renderedTextContrast, withRenderedContrast } from './support/rendered-contrast';

// #348 — the subview sweep's nested-dialog contrast fallback must measure the
// text, not the most contrasting pixel. Low-contrast text next to a black
// border and a black icon has to stay a failure; readable text has to pass
// on its own measured ratio.

const icon = '<svg width="16" height="16" aria-hidden="true"><rect width="16" height="16" fill="#000"/></svg>';

async function nestedDialog(page: Page, color: string) {
  await page.setContent(`<html lang="en"><head><title>Rendered contrast control</title><style>
    body{background:#fff}
    dialog[open]{position:relative;margin:10px;padding:10px;background:#fff;border:0}
    #probe{color:${color};background:#fff;border:2px solid #000;padding:20px;font:16px Arial}
  </style></head><body><dialog open aria-label="outer"><dialog open aria-label="inner">
    <p id="probe">${icon} Contrast control text with a black border and icon</p>
  </dialog></dialog></body></html>`);
}

test('low-contrast text beside a black border and icon stays a contrast failure', async ({ page }) => {
  await nestedDialog(page, '#999');
  const violations = (await accessibilityViolations(page)).filter((v) => v.id === 'color-contrast');
  expect(violations.map((v) => v.targets)).toEqual([['#probe']]);

  const contrast = await renderedTextContrast(page, '#probe');
  expect(contrast?.glyphPixels).toBeGreaterThan(0);
  expect(contrast!.ratio).toBeCloseTo(2.85, 1);

  const notes: string[] = [];
  const kept = await withRenderedContrast(page, violations, notes);
  expect(kept).toHaveLength(1);
  expect(kept[0].summary).toContain(violations[0].summary ?? '');
  expect(kept[0].summary).toMatch(/text #[0-9a-f]{6} on #ffffff 2\.\d\d:1/);
  expect(notes).toEqual([]);
});

test('readable text in a nested dialog passes on its measured text contrast', async ({ page }) => {
  await nestedDialog(page, '#333');
  const contrast = await renderedTextContrast(page, '#probe');
  // #333 on white is 12.63:1; the black border and icon (21:1) must not show.
  expect(contrast!.ratio).toBeCloseTo(12.63, 1);
  expect(contrast!.text).toBe('#333333');

  // As if axe had misjudged the nested dialog's background.
  const notes: string[] = [];
  const misjudged = { id: 'color-contrast', help: 'contrast', nodes: 1, targets: ['#probe'], summary: 'axe: 1.99:1' };
  expect(await withRenderedContrast(page, [misjudged], notes)).toEqual([]);
  expect(notes.join()).toContain('axe: 1.99:1');
  expect(notes.join()).toContain('text #333333 on #ffffff 12.63:1');
});

test('a contrast finding outside a nested dialog is never re-measured', async ({ page }) => {
  await page.setContent('<html lang="en"><head><title>Rendered contrast control</title></head><body><dialog open><p id="probe" style="color:#333">Readable</p></dialog></body></html>');
  const finding = { id: 'color-contrast', help: 'contrast', nodes: 1, targets: ['#probe'], summary: 'axe' };
  const kept = await withRenderedContrast(page, [finding], []);
  expect(kept).toHaveLength(1);
  expect(kept[0].summary).toContain('not nested');
});

for (const [color, ratio] of [['#333', 12.63], ['#999', 2.85]] as const) {
  test(`rendered text measurement respects production style CSP (${color})`, async ({ page }) => {
    await nestedDialog(page, color);
    // Apply the real app's style policy after the fixture's own CSS has
    // loaded. Subsequent injected styles must obey it, as on the real page.
    await page.evaluate(() => {
      const policy = document.createElement('meta');
      policy.httpEquiv = 'Content-Security-Policy';
      policy.content = "style-src 'self'";
      document.head.append(policy);
    });
    const before = await page.locator('#probe').evaluate((node) => [node, ...node.querySelectorAll('*')].map((element) => element.getAttribute('style')));
    const contrast = await renderedTextContrast(page, '#probe');
    expect(contrast?.glyphPixels).toBeGreaterThan(0);
    expect(contrast!.ratio).toBeCloseTo(ratio, 1);
    const after = await page.locator('#probe').evaluate((node) => [node, ...node.querySelectorAll('*')].map((element) => element.getAttribute('style')));
    expect(after).toEqual(before);
    const finding = { id: 'color-contrast', help: 'contrast', nodes: 1, targets: ['#probe'], summary: 'axe: nested background' };
    const notes: string[] = [];
    const kept = await withRenderedContrast(page, [finding], notes);
    expect(kept).toHaveLength(ratio >= 4.5 ? 0 : 1);
  });
}
