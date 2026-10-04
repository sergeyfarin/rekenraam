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
        ...surfaces.flatMap((surface): [string, number][] => [
          [`accent badge on ${surface}`, contrast(p['accent'], over(p['accent'], accentTint, p[surface]))],
          [`accent text on ${surface}`, contrast(p['accent'], p[surface])]
        ])
      ];
      const failing = pairs.filter(([, ratio]) => ratio < AA).map(([name, ratio]) => `${name} ${ratio.toFixed(2)}:1`);
      expect(failing).toEqual([]);
    });
  }
});
