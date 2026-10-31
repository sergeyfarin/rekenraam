import { expect, type Page, type Request } from '@playwright/test';

/** Each pending request keeps the document generation it started in. */
type Activity = { pending: Map<Request, number>; revision: number; generation: number };
const activity = new WeakMap<Page, Activity>();

/** Start before navigation so even requests already pending at the wait are counted. */
export function observePageRequests(page: Page): void {
  if (activity.has(page)) return;
  const state: Activity = { pending: new Map(), revision: 0, generation: 0 };
  const started = (request: Request) => {
    // A main-frame document request starts a new page; client-side navigation
    // loads no document, so its in-flight requests stay awaited.
    if (request.isNavigationRequest() && request.frame() === page.mainFrame()) state.generation++;
    state.pending.set(request, state.generation);
    state.revision++;
  };
  const finished = (request: Request) => {
    if (state.pending.delete(request)) state.revision++;
  };
  // A request the previous document had in flight when it was torn down can
  // miss its requestfailed event (seen with routed requests), and would hold
  // every later wait until it times out (#478's CI). It can no longer change
  // the page, so the new document drops it.
  const loaded = () => {
    for (const [request, generation] of state.pending) {
      if (generation < state.generation) state.pending.delete(request);
    }
    state.revision++;
  };
  page.on('request', started);
  page.on('requestfinished', finished);
  page.on('requestfailed', finished);
  page.on('domcontentloaded', loaded);
  page.once('close', () => {
    page.off('request', started);
    page.off('requestfinished', finished);
    page.off('requestfailed', finished);
    page.off('domcontentloaded', loaded);
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
