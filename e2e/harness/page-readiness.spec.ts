import { expect, test, type Page } from '@playwright/test';
import { observePageRequests, waitForPageReady } from '../playwright/support/page-readiness';

function barrier() {
  let release!: () => void;
  const promise = new Promise<void>(resolve => { release = resolve; });
  return { promise, release };
}

async function frames(page: Page) {
  await page.evaluate(async () => {
    await new Promise<void>(resolve => requestAnimationFrame(() => resolve()));
    await new Promise<void>(resolve => requestAnimationFrame(() => resolve()));
  });
}

test('readiness waits for pending and chained requests before rendered state', async ({ page }) => {
  const first = barrier(), second = barrier(), requestedFirst = barrier(), requestedSecond = barrier();
  observePageRequests(page);
  await page.route('http://readiness.test/**', async route => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname === '/first') {
      requestedFirst.release();
      await first.promise;
      await route.fulfill({ body: 'first' });
    } else if (pathname === '/second') {
      requestedSecond.release();
      await second.promise;
      await route.fulfill({ body: 'second' });
    } else {
      await route.fulfill({ contentType: 'text/html', body: `<h1>pending</h1><script>
        fetch('/first').then(r => r.text()).then(() => fetch('/second')).then(r => r.text())
          .then(() => document.querySelector('h1').textContent = 'complete');
      </script>` });
    }
  });
  let ready: Promise<void> | undefined;
  try {
    await page.goto('http://readiness.test/');
    await requestedFirst.promise;
    let completed = false;
    ready = waitForPageReady(page).then(() => { completed = true; });
    await frames(page);
    expect(completed).toBe(false);
    first.release();
    await requestedSecond.promise;
    await frames(page);
    expect(completed).toBe(false);
    second.release();
    await ready;
    await expect(page.getByRole('heading')).toHaveText('complete');
  } finally {
    first.release(); second.release();
    await ready?.catch(() => {});
  }
});

test('readiness accounts for failed requests', async ({ page }) => {
  observePageRequests(page);
  await page.route('http://readiness.test/**', async route => {
    if (new URL(route.request().url()).pathname === '/failed') await route.abort();
    else await route.fulfill({ contentType: 'text/html', body: `<h1>pending</h1><script>
      fetch('/failed').catch(() => document.querySelector('h1').textContent = 'refused');
    </script>` });
  });
  await page.goto('http://readiness.test/');
  await waitForPageReady(page);
  await expect(page.getByRole('heading')).toHaveText('refused');
});

test('readiness forgets a request the previous document left in flight', async ({ page }) => {
  const held = barrier(), requested = barrier();
  observePageRequests(page);
  await page.route('http://readiness.test/**', async route => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname === '/held') {
      requested.release();
      await held.promise;
      await route.fulfill({ body: 'late' }).catch(() => {});
    } else await route.fulfill({ contentType: 'text/html', body: pathname === '/next'
      ? '<h1>next</h1>'
      : `<h1>first</h1><script>fetch('/held')</script>` });
  });
  try {
    await page.goto('http://readiness.test/');
    await requested.promise;
    await page.goto('http://readiness.test/next');
    await waitForPageReady(page);
    await expect(page.getByRole('heading')).toHaveText('next');
  } finally {
    held.release();
  }
});

test('readiness forgets a request the departing document starts while the next one loads', async ({ page }) => {
  const next = barrier(), requestedNext = barrier(), held = barrier(), requestedHeld = barrier();
  let navigating = false;
  observePageRequests(page);
  await page.route('http://readiness.test/**', async route => {
    const pathname = new URL(route.request().url()).pathname;
    if (pathname === '/poll') {
      // Polls answer at once until the navigation starts; the first after it is held.
      if (!navigating) return route.fulfill({ body: 'ok' });
      requestedHeld.release();
      await held.promise;
      await route.fulfill({ body: 'late' }).catch(() => {});
    } else if (pathname === '/next') {
      navigating = true;
      requestedNext.release();
      await next.promise;
      await route.fulfill({ contentType: 'text/html', body: '<h1>next</h1>' });
    } else await route.fulfill({ contentType: 'text/html', body: `<h1>first</h1><script>
      setInterval(() => fetch('/poll').catch(() => {}), 20);
    </script>` });
  });
  let navigation: Promise<unknown> | undefined;
  try {
    await page.goto('http://readiness.test/');
    // The old document starts a request after the navigation began, before it commits.
    navigation = page.goto('http://readiness.test/next');
    await requestedNext.promise;
    await requestedHeld.promise;
    next.release();
    await navigation;
    await waitForPageReady(page);
    await expect(page.getByRole('heading')).toHaveText('next');
  } finally {
    next.release(); held.release();
    await navigation?.catch(() => {});
  }
});

test('readiness still waits for requests across a client-side navigation', async ({ page }) => {
  const held = barrier(), requested = barrier();
  observePageRequests(page);
  await page.route('http://readiness.test/**', async route => {
    if (new URL(route.request().url()).pathname === '/data') {
      requested.release();
      await held.promise;
      await route.fulfill({ body: 'data' });
    } else await route.fulfill({ contentType: 'text/html', body: `<h1>start</h1><script>
      history.pushState({}, '', '/elsewhere');
      fetch('/data').then(r => r.text()).then(() => document.querySelector('h1').textContent = 'loaded');
    </script>` });
  });
  let ready: Promise<void> | undefined;
  try {
    await page.goto('http://readiness.test/');
    await requested.promise;
    let completed = false;
    ready = waitForPageReady(page).then(() => { completed = true; });
    await frames(page);
    expect(completed).toBe(false);
    held.release();
    await ready;
    await expect(page.getByRole('heading')).toHaveText('loaded');
  } finally {
    held.release();
    await ready?.catch(() => {});
  }
});

test('readiness waits for font completion and fallback layout', async ({ page }) => {
  const font = barrier(), requested = barrier();
  observePageRequests(page);
  await page.route('http://readiness.test/**', async route => {
    if (new URL(route.request().url()).pathname === '/font.woff2') {
      requested.release();
      await font.promise;
      // Invalid bytes deliberately exercise the completed fallback-font path.
      await route.fulfill({ contentType: 'font/woff2', body: 'invalid-font-control' });
    } else await route.fulfill({ contentType: 'text/html', body: `<style>
      @font-face { font-family: ReadinessControl; src: url('/font.woff2'); }
      h1 { font-family: ReadinessControl, sans-serif; }
    </style><h1>Font control</h1>` });
  });
  let ready: Promise<void> | undefined;
  try {
    await page.goto('http://readiness.test/', { waitUntil: 'domcontentloaded' });
    await requested.promise;
    let completed = false;
    ready = waitForPageReady(page).then(() => { completed = true; });
    await frames(page);
    expect(completed).toBe(false);
    font.release();
    await ready;
    expect(await page.evaluate(() => document.fonts.status)).toBe('loaded');
  } finally {
    font.release();
    await ready?.catch(() => {});
  }
});

test('readiness refuses a page whose requests were not observed', async ({ page }) => {
  await expect(waitForPageReady(page)).rejects.toThrow('observePageRequests must run before navigation');
});
