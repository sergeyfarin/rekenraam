import { expect, test, type Locator, type Page } from './support/fixtures';
import { expectNoAccessibilityViolations } from './support/a11y';
import { readyForLedger } from './support/ledger';
import { useDarkTheme } from './support/session';

// #522 / PR #527 review — the selected Due / Templates view must stay readable
// while the pointer is still over it. A shared hover fill once replaced
// bg-selected under text-selected-foreground (1.16:1 light, 1.78:1 dark), and
// class-name assertions passed regardless, so this measures what renders.

const MINIMUM_TEXT_CONTRAST = 4.5;

/** WCAG contrast of the element's computed text colour on its computed background. */
async function computedContrast(button: Locator) {
  return button.evaluate((node) => {
    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = 1;
    const context = canvas.getContext('2d', { willReadFrequently: true })!;
    // The canvas resolves oklch() and friends to sRGB, as the screen shows it.
    const rgba = (color: string) => {
      context.clearRect(0, 0, 1, 1);
      context.fillStyle = color;
      context.fillRect(0, 0, 1, 1);
      return Array.from(context.getImageData(0, 0, 1, 1).data);
    };
    const luminance = ([r, g, b]: number[]) => {
      const [lr, lg, lb] = [r, g, b].map((channel) => {
        const c = channel / 255;
        return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
      });
      return 0.2126 * lr + 0.7152 * lg + 0.0722 * lb;
    };
    const style = getComputedStyle(node);
    const text = rgba(style.color);
    const background = rgba(style.backgroundColor);
    const [high, low] = [luminance(text), luminance(background)].sort((a, b) => b - a);
    return {
      ratio: (high + 0.05) / (low + 0.05),
      text: style.color,
      background: style.backgroundColor,
      // A translucent fill would blend with what is under it; the measurement
      // above is only exact for an opaque one.
      opaqueBackground: background[3] === 255
    };
  });
}

async function expectReadableWhileHovered(page: Page, button: Locator, label: string) {
  await button.click();
  await expect(button).toHaveAttribute('aria-pressed', 'true');
  // The pointer is still over the button after the click; hover it again so
  // the state does not depend on how the click left the mouse.
  await button.hover();
  expect(await button.evaluate((node) => node.matches(':hover')), `${label} is hovered`).toBe(true);
  await expect
    .poll(async () => {
      const contrast = await computedContrast(button);
      expect(contrast.opaqueBackground, `${label} background ${contrast.background}`).toBe(true);
      return contrast.ratio;
    }, { message: `${label}: selected text contrast while hovered` })
    .toBeGreaterThanOrEqual(MINIMUM_TEXT_CONTRAST);
  await expectNoAccessibilityViolations(page, `${label} (hovered)`);
  expect(await button.evaluate((node) => node.matches(':hover')), `${label} still hovered after axe`).toBe(true);
}

for (const theme of ['light', 'dark'] as const) {
  test(`selected recurring view stays readable while hovered (${theme})`, async ({ page }) => {
    await readyForLedger(page);
    await page.goto('/app/planning/recurring');
    if (theme === 'dark') await useDarkTheme(page);
    await expect(page.locator('html')).toHaveAttribute('data-theme', theme);

    const views = page.getByRole('group', { name: 'Recurring views' });
    for (const name of ['Templates', 'Due for review']) {
      await expectReadableWhileHovered(page, views.getByRole('button', { name, exact: true }), `${theme} ${name}`);
    }
  });
}
