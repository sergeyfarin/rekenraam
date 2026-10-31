import { describe, expect, it } from 'vitest';
import { routes } from '#lib/navigation/routes.ts';
import { homeShortcuts, splitShortcuts } from './shortcuts.ts';

describe('home shortcuts', () => {
  it('lead with Add transaction, the only action, and link the rest to registered pages', () => {
    const shortcuts = homeShortcuts();
    expect(shortcuts.map(shortcut => shortcut.id)).toEqual(['add', 'import', 'transactions', 'spending']);
    expect(shortcuts[0].href).toBeUndefined();
    const pages = new Set(routes.filter(route => !route.legacy).map(route => route.route));
    for (const shortcut of shortcuts.slice(1)) expect(pages.has(shortcut.href!)).toBe(true);
  });

  it('show three on phones and list the rest, in order, behind More', () => {
    expect(splitShortcuts(['a', 'b', 'c', 'd', 'e'])).toEqual({ shown: ['a', 'b', 'c'], more: ['d', 'e'] });
    expect(splitShortcuts(['a', 'b'])).toEqual({ shown: ['a', 'b'], more: [] });
  });
});
