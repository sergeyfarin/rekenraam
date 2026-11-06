import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

/**
 * Every theme × base colour × accent palette keeps status and accent text at
 * WCAG AA (4.5:1) on the surfaces it is drawn on. axe only measures the theme
 * a browser test happens to render; this computes every combination straight
 * from app.css, so a token change that breaks one palette fails here by name.
 *
 * It found the StatusBadge warning tone at 3.1:1 and danger at 4.2:1 in the
 * light theme, and accent text near 3:1 in the amber, blue and violet
 * palettes, none of which any rendered axe run had shown.
 */

const AA = 4.5;
const css = readFileSync(new URL('../app.css', import.meta.url), 'utf8');

type RGB = [number, number, number];

function oklchToSRGB(L: number, C: number, h: number): RGB {
  const a = C * Math.cos((h * Math.PI) / 180);
  const b = C * Math.sin((h * Math.PI) / 180);
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  const linear = [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s
  ];
  return linear.map((v) => {
    const x = Math.max(0, v);
    const encoded = x <= 0.0031308 ? 12.92 * x : 1.055 * x ** (1 / 2.4) - 0.055;
    return Math.min(1, Math.max(0, encoded));
  }) as RGB;
}

function luminance([r, g, b]: RGB): number {
  const f = (c: number) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
}

function contrast(a: RGB, b: RGB): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

// A translucent tint composited over what is behind it.
function over(tint: RGB, alpha: number, backdrop: RGB): RGB {
  return tint.map((c, i) => c * alpha + backdrop[i] * (1 - alpha)) as RGB;
}


function tokenBlocks(): Map<string, Record<string, RGB>> {
  const blocks = new Map<string, Record<string, RGB>>();
  for (const [, selector, body] of css.matchAll(/(:root[^{]*)\{([^}]*)\}/g)) {
    const tokens = blocks.get(selector.trim()) ?? {};
    for (const [, name, L, C, h] of body.matchAll(/--color-([a-z-]+):\s*oklch\(([\d.]+) ([\d.]+) ([\d.]+)\)/g)) {
      tokens[name] = oklchToSRGB(Number(L), Number(C), Number(h));
    }
    blocks.set(selector.trim(), tokens);
  }
  return blocks;
}

const blocks = tokenBlocks();
const shellTint = Number(/\.shell-bar\s*\{[^}]*var\(--color-background\) (\d+)%/.exec(css)?.[1]) / 100;
const accentTint = Number(/\.status-accent-soft\s*\{[^}]*var\(--color-accent\) (\d+)%/.exec(css)?.[1]) / 100;
const surfaces = ['background', 'surface', 'surface-strong', 'toolbar', 'control', 'row-hover'];

function palette(theme: 'light' | 'dark', base: string, accent: string): Record<string, RGB> {
  const layers = [':root'];
  if (theme === 'dark') layers.push(":root[data-theme='dark']");
  if (base !== 'default') {
    layers.push(`:root[data-base-color='${base}']`);
    if (theme === 'dark') layers.push(`:root[data-theme='dark'][data-base-color='${base}']`);
  }
  if (accent !== 'default') {
    layers.push(`:root[data-accent-color='${accent}']`);
    if (theme === 'dark') layers.push(`:root[data-theme='dark'][data-accent-color='${accent}']`);
  }
  return Object.assign({}, ...layers.map((layer) => {
    const tokens = blocks.get(layer);
    if (!tokens) throw new Error(`app.css has no ${layer} block`);
    return tokens;
  }));
}

const combinations = (['light', 'dark'] as const).flatMap((theme) =>
  ['default', 'slate', 'zinc'].flatMap((base) =>
    ['default', 'blue', 'violet', 'rose', 'amber'].map((accent) => ({ theme, base, accent }))
  )
);

describe('theme contrast', () => {
  it('reads the accent tint from app.css', () => {
    expect(accentTint).toBeGreaterThan(0);
    expect(shellTint).toBeGreaterThan(0);
    expect(combinations).toHaveLength(30);
  });

  for (const { theme, base, accent } of combinations) {
    it(`${theme} / ${base} / ${accent} keeps badge and accent text at AA`, () => {
      const p = palette(theme, base, accent);
      const pairs: [string, number][] = [
        ['neutral badge', contrast(p['muted'], p['surface-strong'])],
        ['warning badge', contrast(p['warning'], p['warning-soft'])],
        ['danger badge', contrast(p['danger'], p['danger-soft'])],
        ['positive badge', contrast(p['positive'], p['positive-soft'])],
        ['button text on accent', contrast(p['accent-foreground'], p['accent'])],
        ['selected text', contrast(p['selected-foreground'], p['selected'])],
        // Confirmation buttons carry the page background on their fill (#324).
        ['button text on danger', contrast(p['background'], p['danger'])],
        ['button text on warning', contrast(p['background'], p['warning'])],
        // Hovered danger buttons fade to 90% over the surface. Warning buttons
        // ring instead: the same fade read 4.2:1 in light (#341).
        ['button text on danger, hovered', contrast(over(p['background'], 0.9, p['surface']), over(p['danger'], 0.9, p['surface']))],
        ['note text on warning-soft', contrast(p['foreground'], p['warning-soft'])],
        ...['surface', 'background', 'surface-strong'].flatMap((surface): [string, number][] => [
          [`positive text on ${surface}`, contrast(p['positive'], p[surface])],
          [`danger text on ${surface}`, contrast(p['danger'], p[surface])]
        ]),
        ...surfaces.flatMap((surface): [string, number][] => [
          [`accent badge on ${surface}`, contrast(p['accent'], over(p['accent'], accentTint, p[surface]))],
          [`accent text on ${surface}`, contrast(p['accent'], p[surface])]
        ])
      ];
      // The sticky shell bar is translucent: page content scrolls beneath it,
      // so its muted tab text must hold AA over any opaque token fill there
      // (#501: a bg-foreground control under an 84% tint read 4.41:1).
      for (const [name, backdrop] of Object.entries(p)) {
        pairs.push([`muted shell text over ${name}`, contrast(p['muted'], over(p['background'], shellTint, backdrop))]);
      }
      const failing = pairs.filter(([, ratio]) => ratio < AA).map(([name, ratio]) => `${name} ${ratio.toFixed(2)}:1`);
      expect(failing).toEqual([]);
    });
  }
});

/**
 * New transaction's mode accents (#506). The accent is never text: it draws
 * the selected tab's underline and icon (non-text contrast, 3:1) on the tab's
 * soft fill and on the surface around it, and the tab's label stays in the
 * foreground colour on that fill (AA). Each mode has its own tokens, apart
 * from the error colour and from gain and loss.
 */
const NON_TEXT = 3;
const entryModes = ['out', 'in', 'transfer', 'investment'];

describe('entry mode accents', () => {
  for (const { theme, base, accent } of combinations) {
    it(`${theme} / ${base} / ${accent} keeps tab indicators at 3:1 and labels at AA`, () => {
      const p = palette(theme, base, accent);
      const failing: string[] = [];
      for (const mode of entryModes) {
        const [mark, soft] = [p[`entry-${mode}`], p[`entry-${mode}-soft`]];
        expect(mark, mode).toBeDefined();
        const pairs: [string, number, number][] = [
          [`${mode} indicator on its soft fill`, contrast(mark, soft), NON_TEXT],
          [`label on ${mode} soft fill`, contrast(p['foreground'], soft), AA],
          ...['surface', 'background', 'control'].map((surface): [string, number, number] =>
            [`${mode} indicator on ${surface}`, contrast(mark, p[surface]), NON_TEXT])
        ];
        for (const [name, ratio, floor] of pairs) if (ratio < floor) failing.push(`${name} ${ratio.toFixed(2)}:1`);
      }
      expect(failing).toEqual([]);
    });
  }

  it('gives each mode its own tokens, apart from danger, positive and gain/loss', () => {
    for (const layer of [':root', ":root[data-theme='dark']"]) {
      // Every regex metacharacter escaped, the backslash included (CodeQL js/incomplete-sanitization).
      const selector = layer.replace(/[\\^$.*+?()[\]{}|]/g, '\\$&');
      const body = new RegExp(`${selector}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? '';
      const value = (name: string) => new RegExp(`--color-${name}:\\s*([^;]+);`).exec(body)?.[1];
      const values = entryModes.map((mode) => value(`entry-${mode}`));
      expect(values.every(Boolean), layer).toBe(true);
      expect(new Set(values).size, layer).toBe(entryModes.length);
      for (const other of ['danger', 'positive', 'accent']) expect(values, `${layer} ${other}`).not.toContain(value(other));
    }
  });
});
