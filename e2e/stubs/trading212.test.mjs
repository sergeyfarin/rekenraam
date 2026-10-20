import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { createInterface } from 'node:readline';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

test('provider stub keeps exception diagnostics out of HTTP responses', { timeout: 20_000 }, async (t) => {
  const child = spawn(process.execPath, [fileURLToPath(new URL('./trading212.mjs', import.meta.url))], {
    env: { ...process.env, TRADING212_STUB_PORT: '0' },
    stdio: ['ignore', 'pipe', 'inherit']
  });
  const lines = createInterface({ input: child.stdout });
  t.after(async () => {
    lines.close();
    if (child.exitCode === null && child.signalCode === null) {
      const closed = once(child, 'close');
      child.kill();
      await closed;
    }
  });
  const [ready] = await once(lines, 'line', { signal: AbortSignal.timeout(10_000) });
  const baseURL = new URL(ready.match(/http:\/\/\S+/)[0]).origin;
  const controlURL = `${baseURL}/__control/accounts/synthetic-key`;

  async function expectGenericFailure(response) {
    assert.equal(response.status, 500);
    assert.equal(response.headers.get('content-type'), 'application/json');
    assert.deepEqual(await response.json(), { error: 'internal server error' });
  }

  await t.test('malformed JSON does not echo parser diagnostics or input', async () => {
    await expectGenericFailure(await fetch(controlURL, {
      method: 'PUT', body: 'synthetic-private-marker-not-json'
    }));
  });

  await t.test('malformed account encoding does not expose URI errors', async () => {
    await expectGenericFailure(await fetch(`${baseURL}/__control/accounts/%FF`, {
      method: 'PUT', body: '{}'
    }));
  });

  await t.test('provider transformation failures do not expose type errors', async () => {
    const seed = await fetch(controlURL, {
      method: 'PUT', body: JSON.stringify({ orders: [{ filledAt: null }, { filledAt: null }] })
    });
    assert.equal(seed.status, 204);
    await expectGenericFailure(await fetch(`${baseURL}/api/v0/equity/history/orders`, {
      headers: { Authorization: 'synthetic-key' }
    }));
  });

  await t.test('health, authorization and valid provider requests survive failures', async () => {
    assert.deepEqual(await (await fetch(`${baseURL}/healthz`)).json(), { ok: true });
    const unauthorized = await fetch(`${baseURL}/api/v0/equity/account/summary`);
    assert.equal(unauthorized.status, 401);
    assert.deepEqual(await unauthorized.json(), { error: 'unauthorized' });
    const seed = await fetch(controlURL, {
      method: 'PUT', body: JSON.stringify({ dividends: [{ paidOn: '2026-01-02' }, { paidOn: '2026-01-03' }] })
    });
    assert.equal(seed.status, 204);
    assert.equal(await seed.text(), '');
    const history = await fetch(`${baseURL}/api/v0/equity/history/dividends`, {
      headers: { Authorization: 'synthetic-key' }
    });
    assert.equal(history.status, 200);
    assert.deepEqual(await history.json(), {
      items: [{ paidOn: '2026-01-03' }, { paidOn: '2026-01-02' }], nextPagePath: null
    });
  });
});
