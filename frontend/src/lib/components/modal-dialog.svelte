<script lang="ts">
  import { onMount, type Snippet } from 'svelte';
  import { captureOpener, returnFocus, scopeBusy, wrapTab } from './modal-focus.ts';

  // The app's one modal dialog (#327). Mount it to open it and unmount it to
  // close it. The native modal <dialog> moves focus inside, makes the page
  // behind it inert and turns Escape into a `cancel` event; this component
  // routes that to onCancel unless pending, and returns focus to the opener.
  // The focus half, including wrapping Tab (a modal <dialog> lets it leave
  // for the browser's own controls), is shared with the phone-width side
  // panels in modal-focus.ts.
  // Initial focus is the first focusable descendant, or the one marked
  // `data-initial-focus` (destructive dialogs keep it off the destructive
  // button). A form inside that reports aria-busy counts as pending, so a
  // write in flight is never dismissed from under its owner.
  let {
    role = 'dialog',
    labelledby,
    label,
    describedby,
    pending = false,
    onCancel,
    closeOnBackdrop = false,
    fallbackFocus,
    class: className = '',
    children
  }: {
    role?: 'dialog' | 'alertdialog';
    labelledby?: string;
    label?: string;
    describedby?: string;
    /** While true, Escape and backdrop clicks are ignored. */
    pending?: boolean;
    /** Escape and backdrop clicks call this; without it they do nothing. */
    onCancel?: () => void;
    closeOnBackdrop?: boolean;
    /** Where focus goes on close when the opener has left the page. */
    fallbackFocus?: () => HTMLElement | null | undefined;
    class?: string;
    children: Snippet;
  } = $props();

  let dialog: HTMLDialogElement;
  let closing = false;

  onMount(() => {
    const opener = captureOpener();
    dialog.showModal();
    dialog.querySelector<HTMLElement>('[data-initial-focus]')?.focus();
    return () => {
      closing = true;
      if (dialog.open) dialog.close();
      returnFocus(opener, fallbackFocus);
    };
  });

  function busy() {
    return pending || scopeBusy(dialog);
  }

  function cancel(event: Event) {
    event.preventDefault();
    if (!busy()) onCancel?.();
  }

  // Chromium may close a dialog without a cancelable `cancel` when Escape is
  // pressed repeatedly without other user activation. Keep it open unless the
  // owner agrees to close.
  // The dialog reopens before the owner hears of it, so anything the owner
  // opens in reply (a discard confirmation, #508) stacks above it and keeps
  // focus; an owner that agrees unmounts it.
  function closed() {
    if (closing) return;
    dialog.showModal();
    if (!busy()) onCancel?.();
  }

  function backdropClick(event: MouseEvent) {
    if (closeOnBackdrop && event.target === dialog) cancel(event);
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<dialog
  bind:this={dialog}
  role={role === 'alertdialog' ? 'alertdialog' : undefined}
  aria-modal="true"
  aria-labelledby={labelledby}
  aria-label={label}
  aria-describedby={describedby}
  oncancel={cancel}
  onclose={closed}
  onclick={backdropClick}
  onkeydown={(event) => wrapTab(dialog, event)}
  class="m-auto max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] overflow-y-auto rounded-(--radius-panel) border border-border bg-surface text-foreground shadow-(--shadow-panel) backdrop:bg-background/70 backdrop:backdrop-blur-sm {className}"
>
  {@render children()}
</dialog>
