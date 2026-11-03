import { m } from '#lib/paraglide/messages.js';

/**
 * Home's shortcut tiles (#477). The set is fixed until personalization (#438)
 * lets the owner move, rename, remove and add them; an account register tile
 * joins once pins (#442) exist. Add transaction leads; it and Transfer (#481)
 * are the actions, opening the shared entry dialog; the rest are links.
 */
export type ShortcutID = 'add' | 'transfer' | 'import' | 'transactions' | 'spending';

export type Shortcut = { id: ShortcutID; label: string; href?: string };

/** Phones show this many tiles and list the rest behind More. */
export const phoneShortcutCount = 3;

export function homeShortcuts(): Shortcut[] {
  return [
    { id: 'add', label: m.transactions_form_create_title() },
    { id: 'transfer', label: m.home_shortcut_transfer() },
    { id: 'import', label: m.home_shortcut_import(), href: '/app/ledger/import' },
    { id: 'transactions', label: m.transactions_page_title(), href: '/app/ledger/transactions' },
    { id: 'spending', label: m.home_shortcut_spending(), href: '/app/reports/spending' }
  ];
}

/** The tiles a phone shows and those its More menu lists, keeping order. */
export function splitShortcuts<T>(shortcuts: readonly T[], shown = phoneShortcutCount): { shown: T[]; more: T[] } {
  return { shown: shortcuts.slice(0, shown), more: shortcuts.slice(shown) };
}
