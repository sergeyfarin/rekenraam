import { readFileSync, readdirSync, statSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { parse } from 'svelte/compiler';
import ts from 'typescript';
import { describe, expect, it } from 'vitest';

/**
 * Every colour utility in the frontend names a semantic token from app.css
 * (#324, found by the #278 audit; extends the #259 badge check to the whole pattern).
 *
 * Tailwind emits nothing for a colour it does not know, so a utility such as
 * `text-destructive` or `text-warning-foreground` compiles silently and leaves
 * the element in its inherited colour. That is how 22 error messages and
 * negative gains lost their danger colour, and how the warning and danger
 * confirmation buttons were drawn in foreground text on their own fill — under
 * 2:1 in the dark theme — without any build or type check noticing. Palette
 * colours (`text-green-700`, `bg-black/50`) bypass the theme the same way.
 *
 * Utilities are read only where classes can live: markup attribute values
 * (except `style`), `class:` directives and string or template literals in
 * scripts and `.ts` modules, including both branches of a dynamic class. CSS,
 * comments and markup text are never read, so `border-radius: 0;` in a
 * component `<style>` block is not a `border-*` colour (#462).
 */

const src = fileURLToPath(new URL('..', import.meta.url));
const css = readFileSync(path.join(src, 'app.css'), 'utf8');
const themeBlock = /@theme inline\s*\{([^}]*)\}/.exec(css)?.[1] ?? '';
const tokens = new Set([...themeBlock.matchAll(/--color-([a-z0-9-]+):/g)].map(([, name]) => name));

const keywords = new Set(['inherit', 'current', 'transparent']);
// Utility prefixes whose value is a colour. Each also has non-colour values
// (sizes, alignment, widths, styles), which the pattern below leaves out. A
// name followed by `:` is a CSS declaration inside a string, not a utility.
const colourUtility =
  /(?<![\w/.-])(?:[a-z0-9[\]=&-]+:)*(text|bg|border(?:-[trblxy])?|ring(?:-offset)?|outline|fill|stroke|divide|decoration|accent|caret|from|via|to|placeholder|shadow)-([a-z][a-z0-9-]*)(?:\/\d+)?(?![\w-]|\s*:)/g;
const nonColour =
  /^(xs|sm|base|md|lg|xl|\d?xl|left|right|center|justify|start|end|wrap|nowrap|balance|pretty|ellipsis|clip|solid|dashed|dotted|double|none|collapse|separate|x|y|t|b|l|r|s|e|fixed|local|scroll|cover|contain|auto|top|bottom|no-repeat|repeat|origin|offset|inset|underline|thin|thick|slice|clone|linear|radial|conic|opacity|ellipsis|px|panel)(-.*)?$/;

type Segment = { start: number; end: number };

// Source ranges that can hold class names: see the comment at the top.
function classSegments(file: string, text: string): Segment[] {
  const segments: Segment[] = [];
  if (file.endsWith('.ts')) {
    const visit = (node: ts.Node) => {
      if (ts.isStringLiteral(node) || ts.isTemplateLiteralToken(node)) segments.push({ start: node.getStart(), end: node.end });
      ts.forEachChild(node, visit);
    };
    visit(ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true));
    return segments;
  }
  const visit = (node: unknown): void => {
    if (Array.isArray(node)) return node.forEach(visit);
    if (!node || typeof node !== 'object') return;
    const { type, start, end, name, value } = node as { type?: string; start: number; end: number; name?: string; value?: unknown };
    if (type === 'StyleSheet' || type === 'StyleDirective') return;
    if (type === 'Attribute' && name === 'style') return;
    if (type === 'Literal' && typeof value === 'string') segments.push({ start, end });
    if (type === 'TemplateElement' || type === 'ClassDirective') segments.push({ start, end });
    if (type === 'Attribute' && Array.isArray(value)) {
      for (const part of value as { type: string; start: number; end: number }[]) if (part.type === 'Text') segments.push(part);
    }
    for (const [key, child] of Object.entries(node)) if (key !== 'metadata' && key !== 'leadingComments' && key !== 'trailingComments') visit(child);
  };
  visit(parse(text, { modern: true }));
  return segments.sort((a, b) => a.start - b.start);
}

function unknownUtilities(file: string, text: string): string[] {
  const unknown: string[] = [];
  for (const { start, end } of classSegments(file, text)) {
    for (const match of text.slice(start, end).matchAll(colourUtility)) {
      const [utility, , value] = match;
      if (tokens.has(value) || keywords.has(value) || nonColour.test(value)) continue;
      const line = text.slice(0, start + match.index).split('\n').length;
      unknown.push(`${file}:${line} ${utility}`);
    }
  }
  return unknown;
}

function sources(directory: string): string[] {
  return readdirSync(directory).flatMap((name) => {
    const file = path.join(directory, name);
    if (statSync(file).isDirectory()) return name === 'paraglide' ? [] : sources(file);
    return /\.(svelte|ts)$/.test(name) && !/\.(test|d)\.ts$/.test(name) ? [file] : [];
  });
}

describe('theme tokens', () => {
  it('reads the theme tokens from app.css', () => {
    expect(tokens.has('danger')).toBe(true);
    expect(tokens.has('surface-strong')).toBe(true);
  });

  it('reads utilities from classes and strings, not CSS, comments or text', () => {
    const component = [
      '<script lang="ts">',
      '  // text-destructive in a comment',
      "  const tone = 'text-green-700';",
      "  const fill = (soft: boolean) => `px-2 ${soft ? 'bg-accent-soft' : 'bg-surface'} text-center`;",
      '</script>',
      '<p class="text-sm text-danger border-b border-border {tone ? \'text-muted\' : \'text-destructive\'}" class:bg-black={tone} style="border-radius: 0">',
      '  text-destructive as copy',
      '</p>',
      '<style>.pill { border-radius: 0; text-align: center; outline-offset: 2px; }</style>'
    ].join('\n');
    expect(unknownUtilities('fixture.svelte', component)).toEqual([
      'fixture.svelte:3 text-green-700',
      'fixture.svelte:4 bg-accent-soft',
      'fixture.svelte:6 text-destructive',
      'fixture.svelte:6 class:bg-black'
    ]);

    const module = [
      '/* bg-subtle in a comment */',
      "export const danger = 'text-destructive focus-visible:outline-foreground';",
      'export const rows = (warn: boolean) => `${warn ? `bg-warning-subtle` : "bg-surface"} rounded-md`;',
      "export const css = 'border-radius: 0';"
    ].join('\n');
    expect(unknownUtilities('fixture.ts', module)).toEqual(['fixture.ts:2 text-destructive', 'fixture.ts:3 bg-warning-subtle']);
  });

  it('uses only colour utilities that name a semantic token', () => {
    const unknown = sources(src).flatMap((file) => unknownUtilities(path.relative(src, file), readFileSync(file, 'utf8')));
    expect(unknown).toEqual([]);
  });

  // Fading a warning fill to 90% opacity over the surface reads 4.2:1 in the
  // light theme (#341); warning buttons show hover without changing colour.
  it('never fades a warning-filled control on hover', () => {
    const faded: string[] = [];
    for (const file of sources(src)) {
      readFileSync(file, 'utf8').split('\n').forEach((line, index) => {
        if (/(?<![\w-])bg-warning(?![\w/-])/.test(line) && /hover:opacity-/.test(line)) faded.push(`${path.relative(src, file)}:${index + 1}`);
      });
    }
    expect(faded).toEqual([]);
  });
});
