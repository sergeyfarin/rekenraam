import { m } from '#lib/paraglide/messages.js';

/**
 * Home's shortcut tiles (#477). The set is fixed until personalization (#438)
 * lets the owner move, rename, remove and add them; Transfer and an account
 * register tile join once a transfer entry and pins (#442) exist. Add
 * transaction leads and is the one action; the rest are links.
 */
export type ShortcutID = 'add' | 'import' | 'transactions' | 'spending';

export type Shortcut = { id: ShortcutID; label: string; href?: string };

/** Phones show this many tiles and list the rest behind More. */
export const phoneShortcutCount = 3;

export function homeShortcuts(): Shortcut[] {
  return [
    { id: 'add', label: m.transactions_form_create_title() },
    { id: 'import', label: m.home_shortcut_import(), href: '/app/ledger/import' },
    { id: 'transactions', label: m.transactions_page_title(), href: '/app/ledger/transactions' },
    { id: 'spending', label: m.home_shortcut_spending(), href: '/app/reports/spending' }
  ];
}

/** The tiles a phone shows and those its More menu lists, keeping order. */
export function splitShortcuts<T>(shortcuts: readonly T[], shown = phoneShortcutCount): { shown: T[]; more: T[] } {
  return { shown: shortcuts.slice(0, shown), more: shortcuts.slice(shown) };
}
