import type { Attachment } from 'svelte/attachments';

/**
 * Closes a header disclosure (phone menu, settings menu) the way a menu
 * button is expected to close: Escape returns focus to its button, and a click
 * or focus outside both the button and the panel closes it quietly.
 *
 * Attach to the element wrapping both the button and the panel.
 */
export function dismissible(isOpen: () => boolean, close: (restoreFocus: boolean) => void): Attachment<HTMLElement> {
  return (root) => {
    const onKeydown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.key !== 'Escape' || !isOpen()) return;
      event.preventDefault();
      close(true);
    };
    const onPointerdown = (event: PointerEvent) => {
      if (isOpen() && !root.contains(event.target as Node)) close(false);
    };
    const onFocusin = (event: FocusEvent) => {
      if (isOpen() && !root.contains(event.target as Node)) close(false);
    };

    root.addEventListener('keydown', onKeydown);
    document.addEventListener('pointerdown', onPointerdown);
    document.addEventListener('focusin', onFocusin);
    return () => {
      root.removeEventListener('keydown', onKeydown);
      document.removeEventListener('pointerdown', onPointerdown);
      document.removeEventListener('focusin', onFocusin);
    };
  };
}
