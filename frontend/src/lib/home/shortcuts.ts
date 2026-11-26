import { m } from '#lib/paraglide/messages.js';

import type { HomeLayoutAccount, HomeLayoutShortcut } from '#lib/api/home.ts';
import { layoutAccountLabel } from './layout-model.ts';

export type ShortcutID = HomeLayoutShortcut['id'];
export type Shortcut = { id: ShortcutID; label: string; href?: string };

export const phoneShortcutCount = 3;

export function shortcutCatalog(): readonly Shortcut[] {
  return [
    { id: 'add', label: m.transactions_form_create_title() },
    { id: 'transfer', label: m.home_shortcut_transfer() },
    { id: 'import', label: m.home_shortcut_import(), href: '/app/ledger/import' },
    { id: 'transactions', label: m.transactions_page_title(), href: '/app/ledger/transactions' },
    { id: 'spending', label: m.home_shortcut_spending(), href: '/app/reports/spending' },
    { id: 'reconcile', label: m.reconcile_title(), href: '/app/ledger/reconcile' },
    { id: 'account', label: m.home_layout_account_shortcut() }
  ];
}

export function homeShortcuts(items?: readonly HomeLayoutShortcut[], accounts: readonly HomeLayoutAccount[] = []): Shortcut[] {
  const catalog = shortcutCatalog();
  if (!items) return catalog.slice(0, 5);
  return items.flatMap(item => {
    const base = catalog.find(shortcut => shortcut.id === item.id);
    if (!base) return [];
    if (item.id !== 'account') return [{ ...base, label: item.label || base.label }];
    const account = accounts.find(record => record.id === item.account_id);
    if (!account) return [];
    return [{ ...base, label: item.label || layoutAccountLabel(account), href: `/app/ledger/accounts/${account.id}/register` }];
  });
}

/** The tiles a phone shows and those its More menu lists, keeping order. */
export function splitShortcuts<T>(shortcuts: readonly T[], shown = phoneShortcutCount): { shown: T[]; more: T[] } {
  return { shown: shortcuts.slice(0, shown), more: shortcuts.slice(shown) };
}
