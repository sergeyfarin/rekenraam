import { expect, type Frame, type Page, type Request } from '@playwright/test';

/**
 * Each pending request keeps the document generation it started in. A
 * generation is a committed document: until a navigation commits, the old
 * document still owns the page and may start requests of its own.
 */
type Activity = { pending: Map<Request, number>; revision: number; generation: number; document?: Request };
const activity = new WeakMap<Page, Activity>();

function withoutHash(url: string): string {
  return url.split('#')[0];
}

/** Start before navigation so even requests already pending at the wait are counted. */
export function observePageRequests(page: Page): void {
  if (activity.has(page)) return;
  const state: Activity = { pending: new Map(), revision: 0, generation: 0 };
  const started = (request: Request) => {
    if (request.isNavigationRequest() && request.frame() === page.mainFrame()) state.document = request;
    state.pending.set(request, state.generation);
    state.revision++;
  };
  const finished = (request: Request) => {
    // A navigation that fails commits no document.
    if (request === state.document && request.failure()) state.document = undefined;
    if (state.pending.delete(request)) state.revision++;
  };
  // A request the previous document had in flight when it was torn down can
  // miss its requestfailed event (seen with routed requests), and would hold
  // every later wait until it times out (#478's CI). When the next document
  // commits, the old one can no longer change the page, so everything it
  // started, before or during the navigation, is dropped. A client-side
  // navigation commits no document request, so its requests stay awaited.
  const committed = (frame: Frame) => {
    const document = state.document;
    if (frame !== page.mainFrame() || !document || withoutHash(frame.url()) !== withoutHash(document.url())) return;
    state.document = undefined;
    state.generation++;
    for (const [request, generation] of state.pending) {
      if (request === document) state.pending.set(request, state.generation);
      else if (generation < state.generation) state.pending.delete(request);
    }
    state.revision++;
  };
  page.on('request', started);
  page.on('requestfinished', finished);
  page.on('requestfailed', finished);
  page.on('framenavigated', committed);
  page.once('close', () => {
    page.off('request', started);
    page.off('requestfinished', finished);
    page.off('requestfailed', finished);
    page.off('framenavigated', committed);
  });
  activity.set(page, state);
}

/** Await current/chained requests, fonts and two rendered frames, without a fixed quiet delay. */
export async function waitForPageReady(page: Page): Promise<void> {
  const state = activity.get(page);
  if (!state) throw new Error('observePageRequests must run before navigation');
  await expect.poll(async () => {
    const revision = state.revision;
    const fontsReady = await page.evaluate(async () => {
      await new Promise<void>(resolve => requestAnimationFrame(() => resolve()));
      await new Promise<void>(resolve => requestAnimationFrame(() => resolve()));
      return document.fonts.status === 'loaded';
    });
    return fontsReady && state.pending.size === 0 && state.revision === revision;
  }, { timeout: 10_000, intervals: [0], message: 'requests, fonts and rendered state settle' }).toBe(true);
}
