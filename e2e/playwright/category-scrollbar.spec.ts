import process from 'node:process';
import { expect, test } from './support/fixtures';
import { readyForLedger } from './support/ledger';

// Keep the native scrollbar visible in Chromium's headless test browser.
test.use({
  launchOptions: {
    executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE,
    ignoreDefaultArgs: ['--hide-scrollbars']
  }
});

test('dragging the native Category scrollbar keeps the popup open and the input focused', async ({ page }) => {
  await readyForLedger(page);
  await page.setViewportSize({ width: 1280, height: 950 });
  await page.clock.install();
  await page.goto('/app');
  await page.getByRole('list', { name: 'Shortcuts', exact: true }).getByRole('button', { name: 'New transaction', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'New transaction', exact: true });
  const field = dialog.getByRole('combobox', { name: 'Category', exact: true });
  const list = dialog.getByRole('listbox', { name: 'Category', exact: true });
  await field.focus();
  await expect(list).toBeVisible();
  await list.scrollIntoViewIfNeeded();
  const thumb = await list.evaluate(el => {
    const rect = el.getBoundingClientRect();
    const style = getComputedStyle(el);
    const borderLeft = parseFloat(style.borderLeftWidth);
    const borderRight = parseFloat(style.borderRightWidth);
    const gutter = el.offsetWidth - el.clientWidth - borderLeft - borderRight;
    return { gutter, x: rect.right - borderRight - gutter / 2, y: rect.top + 24 };
  });
  expect(thumb.gutter).toBeGreaterThan(0);
  await page.mouse.move(thumb.x, thumb.y);
  await page.mouse.down();
  const scrollBeforeDrag = await list.evaluate(el => el.scrollTop);
  await page.mouse.move(thumb.x, thumb.y + 55, { steps: 6 });
  // Before the fix, the input's blur timer dismissed the popup during the drag.
  await page.clock.fastForward(250);
  await expect(list).toBeVisible();
  expect(await list.evaluate(el => el.scrollTop)).toBeGreaterThan(scrollBeforeDrag);
  await page.mouse.up();
  await expect(field).toBeFocused();
});
