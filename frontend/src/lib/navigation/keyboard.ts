import { isEditableTarget } from '#lib/search/search-model.ts';
import { m } from '#lib/paraglide/messages.js';

// The app's single-key shortcuts (#459). Search owns `/` and Ctrl/⌘ K
// (`searchShortcut`); the shell owns N and `?`. None acts in an editable field
// or with a modifier other than the Shift that types `?`.

export type ShellShortcut = 'add-transaction' | 'help';

export function shellShortcut(event: KeyboardEvent): ShellShortcut | undefined {
  if (event.defaultPrevented || event.isComposing || event.repeat || event.altKey || event.ctrlKey || event.metaKey) return undefined;
  if (isEditableTarget(event.target)) return undefined;
  if (event.key === '?') return 'help';
  // Caps Lock types N without Shift; Shift+N stays free for later shortcuts.
  if (!event.shiftKey && event.key.toLowerCase() === 'n') return 'add-transaction';
  return undefined;
}

/** Keyboard help's rows: alternative key combinations and what they do. */
export function shortcutList(): { keys: string[][]; label: string }[] {
  return [
    { keys: [['/'], [m.keyboard_key_ctrl(), 'K'], ['⌘', 'K']], label: m.keyboard_help_search() },
    { keys: [['N']], label: m.transactions_form_create_title() },
    { keys: [['?']], label: m.keyboard_help_help() },
    { keys: [[m.keyboard_key_escape()]], label: m.keyboard_help_escape() }
  ];
}
