import { accountLabelFromParts } from '#lib/accounts/account-labels.ts';
import type { HomeLayoutAccount, HomeWidgetID } from '#lib/api/home.ts';

export const wideWidgets: readonly HomeWidgetID[] = ['spending', 'net_worth_trend', 'cashflow', 'holdings', 'allocation'];

export function layoutAccountLabel(account: HomeLayoutAccount): string {
  return accountLabelFromParts({ id: account.id, name: account.name, code: account.code, starterKey: account.starter_key, currencyCode: account.currency_code });
}

export function moveHomeItem<T>(items: readonly T[], from: number, to: number): T[] {
  if (from < 0 || from >= items.length || to < 0 || to >= items.length) return [...items];
  const next = [...items];
  const [moved] = next.splice(from, 1);
  next.splice(to, 0, moved);
  return next;
}

export function focusHomeControl(id: string) {
  document.getElementById(id)?.focus();
}
