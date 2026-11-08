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

type OKLab = [number, number, number];

function oklchToOKLab(L: number, C: number, h: number): OKLab {
  return [L, C * Math.cos((h * Math.PI) / 180), C * Math.sin((h * Math.PI) / 180)];
}

function oklchToSRGB(L: number, C: number, h: number): RGB {
  return oklabToSRGB(oklchToOKLab(L, C, h));
}

function oklabToSRGB([L, a, b]: OKLab): RGB {
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


// A token is a literal OKLCH colour, an alias of another token
// (`var(--color-accent)`), or that token mixed with white in OKLab, as the
// dark focus ring is. Aliases resolve after the palette's layers merge, so a
// palette's own accent flows into the place markers and focus ring.
type Token = { oklab: OKLab } | { alias: string; white: number };

function tokenBlocks(): Map<string, Record<string, Token>> {
  const blocks = new Map<string, Record<string, Token>>();
  for (const [, selector, body] of css.matchAll(/(:root[^{]*)\{([^}]*)\}/g)) {
    const tokens = blocks.get(selector.trim()) ?? {};
    for (const [, name, L, C, h] of body.matchAll(/--color-([a-z-]+):\s*oklch\(([\d.]+) ([\d.]+) ([\d.]+)\)/g)) {
      tokens[name] = { oklab: oklchToOKLab(Number(L), Number(C), Number(h)) };
    }
    for (const [, name, alias] of body.matchAll(/--color-([a-z-]+):\s*var\(--color-([a-z-]+)\);/g)) {
      tokens[name] = { alias, white: 0 };
    }
    for (const [, name, alias, share] of body.matchAll(/--color-([a-z-]+):\s*color-mix\(in oklab, var\(--color-([a-z-]+)\) (\d+)%, white\);/g)) {
      tokens[name] = { alias, white: 1 - Number(share) / 100 };
    }
    blocks.set(selector.trim(), tokens);
  }
  return blocks;
}

function resolve(tokens: Record<string, Token>): Record<string, RGB> {
  const white = oklchToOKLab(1, 0, 0);
  const lab = (name: string, seen: string[] = []): OKLab => {
    const token = tokens[name];
    if (!token) throw new Error(`no --color-${name} for ${seen.at(-1) ?? name}`);
    if ('oklab' in token) return token.oklab;
    if (seen.includes(name)) throw new Error(`--color-${name} aliases itself`);
    const base = lab(token.alias, [...seen, name]);
    return base.map((c, i) => c * (1 - token.white) + white[i] * token.white) as OKLab;
  };
  return Object.fromEntries(Object.keys(tokens).map((name) => [name, oklabToSRGB(lab(name))]));
}

const blocks = tokenBlocks();
const rowTint = (row: string) => Number(new RegExp(`\\.shell-${row}-row\\s*\\{[^}]*var\\(--color-shell-${row}\\) (\\d+)%`).exec(css)?.[1]) / 100;
const sectionsTint = rowTint('sections');
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
  return resolve(Object.assign({}, ...layers.map((layer) => {
    const tokens = blocks.get(layer);
    if (!tokens) throw new Error(`app.css has no ${layer} block`);
    return tokens;
  })));
}

const combinations = (['light', 'dark'] as const).flatMap((theme) =>
  ['default', 'slate', 'zinc'].flatMap((base) =>
    ['default', 'blue', 'violet', 'rose', 'amber'].map((accent) => ({ theme, base, accent }))
  )
);

describe('theme contrast', () => {
  it('reads the accent tint from app.css', () => {
    expect(accentTint).toBeGreaterThan(0);
    expect(sectionsTint).toBeGreaterThan(0);
    // The pages row is solid so the current tab joins it without a seam.
    expect(css).toMatch(/\.shell-pages-row\s*\{\s*background-color: var\(--color-shell-pages\);\s*\}/);
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
      // The sticky sections row is translucent: page content scrolls beneath
      // it, so its muted text must hold AA over any opaque token fill there
      // (#501: a bg-foreground control under an 84% tint read 4.41:1).
      for (const [name, backdrop] of Object.entries(p)) {
        pairs.push([`muted section text over ${name}`, contrast(p['muted'], over(p['shell-sections'], sectionsTint, backdrop))]);
      }
      pairs.push(['muted page tab text on pages row', contrast(p['muted'], p['shell-pages'])]);
      // The current section's tab is solid, in the tint it joins (#489).
      pairs.push(['current section on pages tint', contrast(p['foreground'], p['shell-pages'])]);
      pairs.push(['current section on page tint', contrast(p['foreground'], p['background'])]);
      pairs.push(['shell count badge', contrast(p['nav-marker-foreground'], p['nav-marker'])]);
      const failing = pairs.filter(([, ratio]) => ratio < AA).map(([name, ratio]) => `${name} ${ratio.toFixed(2)}:1`);
      expect(failing).toEqual([]);
    });
  }
});

/**
 * Non-text marks in the shell and the focus ring (#489): the current page's
 * underline on the pages row, and the focus ring on every surface a control
 * can sit on, the two shell rows included, at 3:1 (WCAG 1.4.11).
 */
describe('shell place markers and focus ring', () => {
  for (const { theme, base, accent } of combinations) {
    it(`${theme} / ${base} / ${accent} keeps the page underline and focus ring at 3:1`, () => {
      const p = palette(theme, base, accent);
      const pairs: [string, number][] = [
        ['page underline on pages row', contrast(p['nav-marker'], p['shell-pages'])],
        ...['background', 'surface', 'surface-strong', 'control', 'shell-sections', 'shell-pages'].map((surface): [string, number] =>
          [`focus ring on ${surface}`, contrast(p['focus-ring'], p[surface])])
      ];
      const failing = pairs.filter(([, ratio]) => ratio < 3).map(([name, ratio]) => `${name} ${ratio.toFixed(2)}:1`);
      expect(failing).toEqual([]);
    });
  }

  it('defines both shell rows for every theme and base colour', () => {
    // Every block that sets a page background sets the rows drawn over it.
    const checked = [...blocks].filter(([, tokens]) => 'background' in tokens);
    expect(checked.length).toBeGreaterThanOrEqual(7);
    for (const [selector, tokens] of checked) {
      expect(Object.keys(tokens), selector).toEqual(expect.arrayContaining(['shell-pages', 'shell-sections']));
    }
  });
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
