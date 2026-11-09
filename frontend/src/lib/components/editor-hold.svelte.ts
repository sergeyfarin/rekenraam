import { untrack } from 'svelte';
import type { Attachment } from 'svelte/attachments';

// Editors that are open beside the page rather than in a modal: the
// transaction editor in a register's side panel, for instance. Global search
// never opens over one (#490), because choosing a result navigates away and an
// edit or correction has no discard check of its own on navigation.

let held = $state(0);

/** Attach to an editor's root: search stays closed while it is mounted. */
// untrack: reading the count must not make the attachment depend on it, or
// each change would tear it down and run it again.
export const editorHold: Attachment<HTMLElement> = () => {
  untrack(() => { held += 1; });
  return () => { untrack(() => { held -= 1; }); };
};

/** Whether an editor is open; reactive in components. */
export function editorOpen(): boolean {
  return held > 0;
}
