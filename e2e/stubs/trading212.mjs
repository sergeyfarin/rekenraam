/**
 * Trading 212 provider stub for e2e runs (T-128).
 *
 * The app reaches it through TRADING212_BASE_URL, which the backend honors
 * only with APP_ENV=development. It serves the endpoints the fetcher calls —
 * account summary (probe), orders, dividends and cash transactions — in the
 * provider's wire shape, keyed by the API key in the Authorization header so
 * every spec owns its own account. An unregistered key gets 401, exactly as
 * the real provider rejects a bad key.
 *
 * Specs control fills through `PUT /__control/accounts/{key}` with
 * `{ orders: [...], dividends: [...], transactions: [...] }`; each PUT
 * replaces that account's whole history, so a revised fill is the same id
 * with new values. See playwright/support/trading212.ts.
 */

import { createServer } from 'node:http';
import process from 'node:process';

const port = Number(process.env.TRADING212_STUB_PORT ?? '16890');
const apiPrefix = '/api/v0';
/** @type {Map<string, { orders: any[], dividends: any[], transactions: any[] }>} */
const accounts = new Map();

function send(res, status, body) {
  res.writeHead(status, { 'Content-Type': 'application/json' });
  res.end(body === undefined ? '' : JSON.stringify(body));
}

async function readJSON(req) {
  let raw = '';
  for await (const chunk of req) raw += chunk;
  return raw === '' ? {} : JSON.parse(raw);
}

/** Newest first, as the provider pages its history. */
function newestFirst(items, timestampOf) {
  return [...items].sort((a, b) => timestampOf(b).localeCompare(timestampOf(a)));
}

function orderItem(fill) {
  const instrument = { ticker: fill.ticker, isin: fill.isin, name: fill.ticker, currency: fill.currency };
  return {
    order: {
      status: fill.orderStatus ?? 'FILLED', id: fill.orderId ?? fill.id + 1_000_000, ticker: fill.ticker,
      side: fill.side, currency: fill.currency, instrument
    },
    fill: {
      type: fill.type ?? 'TRADE', id: fill.id, filledAt: fill.filledAt, price: Number(fill.price),
      quantity: Number(fill.quantity), walletImpact: { currency: fill.currency, netValue: Number(fill.netValue) }
    }
  };
}

const server = createServer(async (req, res) => {
  try {
    const url = new URL(req.url ?? '/', 'http://stub');
    if (url.pathname === '/healthz') return send(res, 200, { ok: true });

    const control = url.pathname.match(/^\/__control\/accounts\/([^/]+)$/);
    if (control && req.method === 'PUT') {
      const body = await readJSON(req);
      accounts.set(decodeURIComponent(control[1]), {
        orders: body.orders ?? [], dividends: body.dividends ?? [], transactions: body.transactions ?? []
      });
      return send(res, 204);
    }

    if (req.method !== 'GET' || !url.pathname.startsWith(apiPrefix)) return send(res, 404, { error: 'not found' });
    const account = accounts.get(req.headers.authorization ?? '');
    if (!account) return send(res, 401, { error: 'unauthorized' });

    switch (url.pathname.slice(apiPrefix.length)) {
      case '/equity/account/summary':
        return send(res, 200, { currency: 'USD' });
      case '/equity/history/orders':
        return send(res, 200, { items: newestFirst(account.orders, (fill) => fill.filledAt).map(orderItem), nextPagePath: null });
      case '/equity/history/dividends':
        return send(res, 200, { items: newestFirst(account.dividends, (item) => item.paidOn), nextPagePath: null });
      case '/equity/history/transactions':
        return send(res, 200, { items: newestFirst(account.transactions, (item) => item.dateTime), nextPagePath: null });
      default:
        return send(res, 404, { error: 'not found' });
    }
  } catch (err) {
    send(res, 500, { error: String(err) });
  }
});

server.listen(port, '127.0.0.1', () => {
  console.log(`trading212 stub listening on http://127.0.0.1:${port}${apiPrefix}`);
});
