/**
 * Rendered text contrast for axe colour-contrast findings in nested dialogs.
 *
 * axe sometimes blends a dialog opened inside another dialog with a modal
 * backdrop that is not over it (seen on the reconciliation review inside a
 * correction: 1.99:1 reported, about 5:1 on screen). A colour-contrast finding
 * whose every node sits in a nested open dialog is therefore measured again
 * from the rendered pixels, and it stays a failure below 4.5:1.
 *
 * The measurement must find the text itself (#348). Each node is screenshotted
 * twice: as rendered, and with its own text fill made transparent. The pixels
 * that differ are the node's glyphs, and the second shot holds the background
 * actually rendered under each of them. Borders, icons and child elements are
 * the same in both shots, so they cannot stand in for the text. A node's ratio
 * is the highest glyph pixel against the background under it: anti-aliased
 * edge pixels lie between the two colours, so the solid glyph core is the text
 * colour and no pixel can overstate it.
 */

import type { Page } from '@playwright/test';
import type { AxeViolation } from './a11y';

/** WCAG AA for normal text. Large text's 3:1 is not applied: nodes are not sized here. */
export const MINIMUM_TEXT_CONTRAST = 4.5;

export type TextContrast = { ratio: number; text: string; background: string; glyphPixels: number };

/** The text-only contrast of the node `selector` names, or null when it renders no text pixels. */
export async function renderedTextContrast(page: Page, selector: string): Promise<TextContrast | null> {
  const element = page.locator(selector).first();
  await page.mouse.move(0, 0);
  const rendered = (await element.screenshot({ animations: 'disabled' })).toString('base64');
  // CSSOM property writes work under production style-src 'self'; an inline
  // stylesheet is blocked (#361). Preserve each descendant's actual fill so
  // only the target's own glyphs differ, never its child text/icons/borders.
  const saved = await element.evaluateHandle((node) => {
    const nodes = [node, ...node.querySelectorAll<HTMLElement | SVGElement>('*')];
    const styles = nodes.map((item) => ({
      item,
      hadStyle: item.hasAttribute('style'),
      value: item.style.getPropertyValue('-webkit-text-fill-color'),
      priority: item.style.getPropertyPriority('-webkit-text-fill-color'),
      computed: getComputedStyle(item).getPropertyValue('-webkit-text-fill-color')
    }));
    styles.forEach(({ item, computed }, index) => item.style.setProperty('-webkit-text-fill-color', index === 0 ? 'transparent' : computed, 'important'));
    return styles;
  });
  let blank: string;
  try {
    blank = (await element.screenshot({ animations: 'disabled' })).toString('base64');
  } finally {
    try {
      await saved.evaluate((styles) => {
        styles.forEach(({ item, value, priority, hadStyle }) => {
          if (value) item.style.setProperty('-webkit-text-fill-color', value, priority);
          else item.style.removeProperty('-webkit-text-fill-color');
          if (!hadStyle && item.getAttribute('style') === '') item.removeAttribute('style');
        });
      });
    } finally {
      await saved.dispose();
    }
  }
  return page.evaluate(async ([withText, withoutText]) => {
    const pixelsOf = async (data: string) => {
      const image = new Image();
      image.src = `data:image/png;base64,${data}`;
      await image.decode();
      const canvas = document.createElement('canvas');
      canvas.width = image.width; canvas.height = image.height;
      const context = canvas.getContext('2d')!;
      context.drawImage(image, 0, 0);
      return context.getImageData(0, 0, image.width, image.height).data;
    };
    const [text, background] = await Promise.all([pixelsOf(withText), pixelsOf(withoutText)]);
    if (text.length !== background.length) throw new Error('rendered contrast: the node changed size without its text');
    const lum = (p: Uint8ClampedArray, i: number) => [0, 1, 2].map((c) => p[i + c] / 255)
      .map((c) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
      .reduce((sum, c, k) => sum + c * [0.2126, 0.7152, 0.0722][k], 0);
    const hex = (p: Uint8ClampedArray, i: number) => `#${[0, 1, 2].map((c) => p[i + c].toString(16).padStart(2, '0')).join('')}`;
    let best: { ratio: number; text: string; background: string } | null = null;
    let glyphPixels = 0;
    for (let i = 0; i < text.length; i += 4) {
      if (text[i] === background[i] && text[i + 1] === background[i + 1] && text[i + 2] === background[i + 2]) continue;
      glyphPixels++;
      const [hi, lo] = [lum(text, i), lum(background, i)].sort((x, y) => y - x);
      const ratio = (hi + 0.05) / (lo + 0.05);
      if (!best || ratio > best.ratio) best = { ratio, text: hex(text, i), background: hex(background, i) };
    }
    return best && { ...best, glyphPixels };
  }, [rendered, blank] as const);
}

/**
 * Returns `violations` without the colour-contrast findings whose every node is
 * in a nested open dialog and whose rendered text measures at least 4.5:1;
 * each one dropped leaves a note with axe's own diagnostics and the measurement.
 */
export async function withRenderedContrast(page: Page, violations: AxeViolation[], notes: string[]): Promise<AxeViolation[]> {
  const kept: AxeViolation[] = [];
  for (const violation of violations) {
    if (violation.id !== 'color-contrast' || violation.nodes !== violation.targets.length) { kept.push(violation); continue; }
    const measured: string[] = [];
    let passes = true;
    for (const target of violation.targets) {
      const nested = await page.locator(target).first()
        .evaluate((node) => !!node.closest('dialog[open]')?.parentElement?.closest('dialog[open]')).catch(() => false);
      if (!nested) { measured.push(`${target}: not nested`); passes = false; break; }
      const contrast = await renderedTextContrast(page, target);
      if (!contrast) { measured.push(`${target}: no text pixels`); passes = false; continue; }
      measured.push(`${target}: text ${contrast.text} on ${contrast.background} ${contrast.ratio.toFixed(2)}:1 (${contrast.glyphPixels} px)`);
      if (contrast.ratio < MINIMUM_TEXT_CONTRAST) passes = false;
    }
    if (passes) notes.push(`axe color-contrast in a nested dialog [${violation.summary ?? ''}] re-measured from rendered text: ${measured.join(', ')}`);
    else kept.push({ ...violation, summary: `${violation.summary ?? ''} (rendered: ${measured.join(', ')})` });
  }
  return kept;
}
