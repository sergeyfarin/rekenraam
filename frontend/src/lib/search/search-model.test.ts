import { describe, expect, it, vi } from 'vitest';
import { applePlatform, createSearchIndex, searchResults, normalizeSearch, searchShortcut, searchShortcutHint, type SearchResult } from './search-model.ts';

const items: SearchResult[] = [
  { id: 'a2', group: 'account', title: 'Café savings' },
  { id: 'a1', group: 'account', title: 'Café' },
  { id: 'a3', group: 'account', title: 'Old Café', inactive: true },
  { id: 'p', group: 'page', title: 'Café report' },
  { id: 'transactions', group: 'page', title: 'Transactions' },
  { id: 'c', group: 'category', title: 'Food', keywords: 'Café' },
  { id: 'i', group: 'instrument', title: 'ACME.B', detail: 'ACMB' },
  { id: 'cmd', group: 'command', title: 'Add transaction' }
];
const index = createSearchIndex(items);

describe('shared search ranking and grouping', () => {
  it('recognizes slash on layouts requiring Shift, and preserves modified/composing shortcuts', () => {
    const event = (overrides: Partial<KeyboardEvent>) => ({ key: '/', target: null, ...overrides }) as KeyboardEvent;
    expect(searchShortcut(event({ shiftKey: true }))).toBe(true);
    expect(searchShortcut(event({ key: 'k', ctrlKey: true }))).toBe(true);
    expect(searchShortcut(event({ key: 'k', metaKey: true }))).toBe(true);
    for (const overrides of [{ altKey: true }, { isComposing: true }, { repeat: true }, { defaultPrevented: true }, { ctrlKey: true }]) {
      expect(searchShortcut(event(overrides))).toBe(false);
    }
    expect(searchShortcut(event({ key: 'k', ctrlKey: true, shiftKey: true }))).toBe(false);
  });

  it('ignores single-key slash while typing, but Ctrl/⌘ K works from ordinary fields', () => {
    class FakeElement {
      isContentEditable = false;
      closest() { return this; }
    }
    vi.stubGlobal('HTMLElement', FakeElement);
    try {
      const field = new FakeElement();
      expect(searchShortcut({ key: '/', target: field } as unknown as KeyboardEvent)).toBe(false);
      expect(searchShortcut({ key: 'k', ctrlKey: true, target: field } as unknown as KeyboardEvent)).toBe(true);
      expect(searchShortcut({ key: 'K', metaKey: true, target: field } as unknown as KeyboardEvent)).toBe(true);
    } finally { vi.unstubAllGlobals(); }
  });

  it('shows the header shortcut per platform with a localized Ctrl', () => {
    expect(applePlatform('MacIntel')).toBe(true);
    expect(applePlatform('macOS')).toBe(true);
    expect(applePlatform('iPad')).toBe(true);
    expect(applePlatform('Win32')).toBe(false);
    expect(applePlatform('Linux x86_64')).toBe(false);
    expect(searchShortcutHint(true, 'Strg')).toEqual({ label: '⌘ K', keys: 'Meta+K' });
    expect(searchShortcutHint(false, 'Strg')).toEqual({ label: 'Strg K', keys: 'Control+K' });
  });

  it('uses fixed groups and exact names before prefix/keyword matches', () => {
    expect(searchResults(items, index, 'cafe').map(item => item.id)).toEqual(['p', 'a1', 'a2', 'c']);
  });
  it('supports scoped empty search and explicit inactive inclusion', () => {
    expect(searchResults(items, index, '', 'account').map(item => item.id)).toEqual(['a1', 'a2']);
    expect(searchResults(items, index, 'cafe', 'account', true).map(item => item.id)).toEqual(['a1', 'a2', 'a3']);
  });
  it('suggests pages and commands in global empty state', () => {
    expect(searchResults(items, index, '').map(item => item.id)).toEqual(['p', 'transactions', 'cmd']);
  });
  it('handles accents, punctuation, symbols and fuzzy names without interpreting commands', () => {
    expect(normalizeSearch('  CAFÉ  ')).toBe('cafe');
    expect(searchResults(items, index, 'ACME.B').map(item => item.id)).toEqual(['i']);
    expect(searchResults(items, index, 'ACMB').map(item => item.id)).toEqual(['i']);
    expect(searchResults(items, index, 'Add transacton').map(item => item.id)).toEqual(['cmd']);
    expect(searchResults(items, index, 'Add transaction').map(item => item.id)).toEqual(['cmd']);
    expect(searchResults(items, index, '" OR *')).toEqual([]);
  });
});
