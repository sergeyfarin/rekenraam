import type { Attachment } from 'svelte/attachments';

// The keyboard half of the app's modal contract (#327), shared by
// ModalDialog and the side panels that cover the page at phone width (#342):
// focus moves inside on open, Tab and Shift+Tab wrap inside, Escape cancels
// unless a write is pending, and focus returns to the opener on close.

// The opener of the most recently closed modal. A modal opened from inside
// another one (a reason modal handing over to a reconciliation override)
// often loses its own opener when the first one closes; it then returns focus
// to where that first one would have.
let lastReturnTarget: HTMLElement | null = null;

/** The control a modal opening now should hand focus back to. */
export function captureOpener(): HTMLElement | null {
  const active = document.activeElement;
  return active instanceof HTMLElement && active !== document.body ? active : lastReturnTarget;
}

/** Returns focus to the opener, or to `fallback` when it has left the page. */
export function returnFocus(opener: HTMLElement | null, fallback?: () => HTMLElement | null | undefined) {
  const target = opener?.isConnected ? opener : (fallback?.() ?? null);
  lastReturnTarget = target;
  target?.focus();
}

/** The nearest modal scope an element belongs to: a dialog, or a phone-modal panel. */
function scopeOf(element: Element): Element | null {
  return element.closest('dialog, [data-modal-scope]');
}

/**
 * Whether a modal owns the keyboard now: an open dialog, or a side panel that
 * covers the page below `lg`. Global shortcuts stay out of its way.
 */
export function modalOpen(): boolean {
  return document.querySelector('dialog[open]') !== null
    || (window.matchMedia('(width < 64rem)').matches && document.querySelector('[data-modal-scope]') !== null);
}

/** Whether `event` belongs to `scope` itself rather than a dialog nested in it. */
export function ownsEvent(scope: Element, event: Event): boolean {
  return event.target instanceof Element && scopeOf(event.target) === scope;
}

/** Whether a write inside `scope` is in flight: a form there reports aria-busy. */
export function scopeBusy(scope: Element): boolean {
  return scope.querySelector('[aria-busy="true"]') !== null;
}

const focusableSelector = 'a[href], area[href], button, input:not([type="hidden"]), select, textarea, summary, [tabindex]';

/** Focusable controls of `scope`, excluding those in nested dialogs. */
function focusableIn(scope: Element): HTMLElement[] {
  return [...scope.querySelectorAll<HTMLElement>(focusableSelector)].filter((element) =>
    scopeOf(element) === scope && element.tabIndex >= 0 && !element.matches(':disabled') &&
    element.checkVisibility());
}

/** Moves focus to the control marked `data-initial-focus`, else the first one, else the scope. */
export function focusInitial(scope: HTMLElement) {
  const target = scope.querySelector<HTMLElement>('[data-initial-focus]') ?? focusableIn(scope)[0] ?? scope;
  target.focus();
}

/**
 * Wraps Tab and Shift+Tab inside `scope` so focus never lands on the inert
 * page or the browser's own controls. Events from a nested dialog belong to
 * that dialog.
 */
export function wrapTab(scope: HTMLElement, event: KeyboardEvent) {
  if (event.key !== 'Tab' || event.defaultPrevented || !ownsEvent(scope, event)) return;
  const focusable = focusableIn(scope);
  if (focusable.length === 0) { event.preventDefault(); return; }
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (event.shiftKey && (event.target === first || event.target === scope)) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && (event.target === last || event.target === scope)) {
    event.preventDefault();
    first.focus();
  }
}

/** Makes everything outside `scope` inert; returns the undo. */
function inertOutside(scope: Element): () => void {
  const made: Element[] = [];
  for (let node: Element = scope; node.parentElement && node !== document.body; node = node.parentElement) {
    for (const sibling of node.parentElement.children) {
      if (sibling === node || sibling.hasAttribute('inert') || sibling instanceof HTMLScriptElement) continue;
      sibling.setAttribute('inert', '');
      made.push(sibling);
    }
  }
  return () => { for (const element of made) element.removeAttribute('inert'); };
}

/**
 * A side panel that covers the page below Tailwind's `lg` breakpoint and sits
 * beside the list from `lg` up (#342). While covering, it is modal under the
 * same contract as ModalDialog; beside the list it stays an ordinary column.
 * It keeps its landmark role: inert already hides the covered page from
 * assistive technology, and aria-modal is not valid on a landmark.
 * Attach it to the panel and mount the panel to open it.
 */
export function phoneModal(options: {
  /** Escape calls this unless a write is pending. */
  onCancel: () => void;
  /** A write owned outside the panel's forms (their aria-busy also counts). */
  pending?: () => boolean;
  /** The media query under which the panel covers the page. */
  query?: string;
}): Attachment<HTMLElement> {
  return (scope) => {
    const media = window.matchMedia(options.query ?? '(width < 64rem)');
    const opener = captureOpener();
    let release: (() => void) | null = null;
    scope.dataset.modalScope = '';
    scope.tabIndex = -1;

    function apply() {
      if (media.matches && !release) {
        release = inertOutside(scope);
        if (!scope.contains(document.activeElement)) focusInitial(scope);
      } else if (!media.matches && release) {
        release();
        release = null;
      }
    }

    // On the document, not the panel: when the panel swaps its content (an
    // edit saved back to the detail) the focused control leaves with it and
    // focus falls to <body>, which still belongs to the panel's keyboard.
    function keydown(event: KeyboardEvent) {
      if (!release || !(event.target === document.body || ownsEvent(scope, event))) return;
      if (event.key === 'Escape' && !event.defaultPrevented) {
        event.preventDefault();
        if (!options.pending?.() && !scopeBusy(scope)) options.onCancel();
        return;
      }
      wrapTab(scope, event);
    }

    apply();
    media.addEventListener('change', apply);
    document.addEventListener('keydown', keydown);
    return () => {
      media.removeEventListener('change', apply);
      document.removeEventListener('keydown', keydown);
      if (release) {
        release();
        returnFocus(opener);
      }
    };
  };
}
