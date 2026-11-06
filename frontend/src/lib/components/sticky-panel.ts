import type { Attachment } from 'svelte/attachments';

/**
 * Publishes a desktop side panel's height as `--panel-height`, which the
 * `side-panel-sticky` utility (app.css) uses to stick the panel below the
 * shell bar when it fits the window, and at its own bottom when it does not:
 * the page then scrolls a tall panel up until Save is on screen, keeping one
 * page scroll and no nested scroller.
 */
export const publishPanelHeight: Attachment<HTMLElement> = (panel) => {
  const observer = new ResizeObserver(() => panel.style.setProperty('--panel-height', `${panel.offsetHeight}px`));
  observer.observe(panel);
  return () => observer.disconnect();
};
