/**
 * Keyboard for the editor's suggestion lists (payee, category): the combobox
 * pattern search already follows (#457). Options are never Tab stops — a list
 * that closes on blur would drop focus out of a dialog (#477) — so arrows
 * move an active option, Enter chooses it and Escape closes the list without
 * dismissing the dialog or panel around the editor.
 */
export type SuggestionKeyResult =
  | { kind: 'move'; active: number }
  | { kind: 'choose'; index: number }
  | { kind: 'close' }
  | { kind: 'none' };

export function suggestionKey(key: string, open: boolean, count: number, active: number): SuggestionKeyResult {
  if ((key === 'ArrowDown' || key === 'ArrowUp') && count > 0) {
    const step = key === 'ArrowDown' ? 1 : -1;
    if (!open || active < 0 || active >= count) return { kind: 'move', active: step > 0 ? 0 : count - 1 };
    return { kind: 'move', active: (active + step + count) % count };
  }
  if (key === 'Enter' && open && active >= 0 && active < count) return { kind: 'choose', index: active };
  if (key === 'Escape' && open && count > 0) return { kind: 'close' };
  return { kind: 'none' };
}
