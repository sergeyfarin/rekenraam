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
const dividendFetches = new Map();
const dividendDates = new Map();
const splitDates = new Map();

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

    // Synthetic market-data dates are explicit calendar facts supplied by the
    // spec, never recalculated from this process's clock or timezone (#386).
    const dividendControl = url.pathname.match(/^\/__control\/dividends\/([^/]+)$/);
    if (dividendControl && req.method === 'PUT') {
      const symbol = decodeURIComponent(dividendControl[1]);
      const { ex_date: exDate } = await readJSON(req);
      const parsedDate = new Date(`${exDate}T00:00:00Z`);
      if (!symbol.startsWith('E2EDIV') || typeof exDate !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(exDate)
        || !Number.isFinite(parsedDate.getTime()) || parsedDate.toISOString().slice(0, 10) !== exDate) {
        return send(res, 400, { error: 'invalid dividend fixture' });
      }
      dividendDates.set(symbol, exDate);
      dividendFetches.delete(symbol);
      return send(res, 204);
    }

    const splitControl = url.pathname.match(/^\/__control\/splits\/(E2ESPLIT[A-Z0-9]+)$/);
    if (splitControl && req.method === 'PUT') {
      const { execution_date: date } = await readJSON(req);
      if (typeof date !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(date)) {
        return send(res, 400, { error: 'invalid split fixture date' });
      }
      splitDates.set(splitControl[1], date);
      return send(res, 204);
    }

    const control = url.pathname.match(/^\/__control\/accounts\/([^/]+)$/);
    if (control && req.method === 'PUT') {
      const body = await readJSON(req);
      accounts.set(decodeURIComponent(control[1]), {
        orders: body.orders ?? [], dividends: body.dividends ?? [], transactions: body.transactions ?? []
      });
      return send(res, 204);
    }

    // Twelve Data GET /dividends (#111): a registered E2EDIV symbol pays
    // 0.50 per share on its explicit ex-date. E2EDIVREV is revised to
    // 0.55 from its second fetch on (#254).
    if (req.method === 'GET' && url.pathname === '/market-data/dividends') {
      const symbol = url.searchParams.get('symbol') ?? '';
      if (!dividendDates.has(symbol) || url.searchParams.get('adjust') !== 'false') return send(res, 404, { error: 'not found' });
      const fetches = (dividendFetches.get(symbol) ?? 0) + 1;
      dividendFetches.set(symbol, fetches);
      const exDate = dividendDates.get(symbol);
      const amount = symbol.startsWith('E2EDIVREV') && fetches > 1 ? 0.55 : 0.5;
      return send(res, 200, {
        meta: { symbol, currency: 'USD', mic_code: url.searchParams.get('mic_code') ?? '', exchange_timezone: 'America/New_York' },
        dividends: [{ ex_date: exDate, amount }]
      });
    }

    const massiveTicker = url.pathname.match(/^\/market-data\/v3\/reference\/tickers\/(E2ESPLIT[A-Z0-9]+)$/);
    if (req.method === 'GET' && (massiveTicker || url.pathname === '/market-data/stocks/v1/splits')) {
      if (req.headers.authorization !== 'Bearer e2e-massive') return send(res, 401, { error: 'unauthorized' });
      const ticker = massiveTicker?.[1] ?? url.searchParams.get('ticker');
      const date = splitDates.get(ticker);
      if (!date) return send(res, 404, { error: 'unknown split fixture' });
      if (massiveTicker) {
        return send(res, 200, { status: 'OK', results: {
          ticker, primary_exchange: 'XNAS', market: 'stocks', locale: 'us', share_class_figi: `FIGI-${ticker}`
        } });
      }
      const results = date < url.searchParams.get('execution_date.gte') || date > url.searchParams.get('execution_date.lte')
        ? [] : [{ id: `${ticker}-split`, ticker, adjustment_type: 'forward_split', execution_date: date, split_from: 1, split_to: 4 }];
      return send(res, 200, { status: 'OK', results });
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
  } catch {
    send(res, 500, { error: 'internal server error' });
  }
});

server.listen(port, '127.0.0.1', () => {
  console.log(`trading212 stub listening on http://127.0.0.1:${server.address().port}${apiPrefix}`);
});
