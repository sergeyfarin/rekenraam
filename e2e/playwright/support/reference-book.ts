/**
 * Browser-side reader of the #266 reference book (fixtures/reference-book.json).
 *
 * It replays the same public /api/v1 steps as the Go reader
 * (backend/internal/api/reference_book_test.go), from inside the page so the
 * browser's own session makes them. The e2e database is shared within a run,
 * so every name and instrument code carries the run's suffix, and the register
 * figures are only meaningful for reads scoped to the book's own accounts.
 */

import { readFile } from 'node:fs/promises';
import type { Page } from '@playwright/test';
import { apiJSON, csrfTokenFor } from './api';
import { ensureCurrency, readyForLedger } from './ledger';

type Step = {
  why: string;
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH';
  path?: string;
  status?: number;
  body?: unknown;
  capture?: Record<string, string>;
  reconcile?: {
    account: string;
    commodity: string;
    statement_date: string;
    statement_balance_value: string;
    statement_balance_scale: number;
  };
};

export type ReferenceBook = {
  steps: Step[];
  register: {
    period: { start_date: string; end_date: string };
    cashflow: {
      accounts: string[];
      rows: Array<Record<string, string>>;
    };
    spending: {
      accounts: string[];
      expense: Array<{ category: string; commodity: string; amount: string }>;
      income: Array<{ category: string; commodity: string; amount: string }>;
    };
    net_worth: Array<{
      as_of: string;
      totals: Array<{ commodity: string; value: string }>;
      converted_eur: string | null;
    }>;
    gains: {
      valuation_as_of: string;
      methods: Array<{
        method: string;
        realized: Array<{ account: string; disposal_date: string; basis?: string; proceeds?: string; gain: string }>;
        realized_total: string;
        unrealized: Array<{ account: string; gain: string | null }>;
      }>;
    };
    [section: string]: unknown;
  };
};

type CreatedTransaction = {
  id: number;
  transaction_date: string;
  description?: string;
};

export type SeededReferenceBook = {
  book: ReferenceBook;
  /** Every key the book names, bound to the id this run created. */
  ids: Map<string, number>;
  suffix: string;
  /** IDs returned by this replay's writes, independent of shared list pagination. */
  transactions: { ordinary: CreatedTransaction[]; investment: CreatedTransaction[] };
};

export async function loadReferenceBook(): Promise<ReferenceBook> {
  return JSON.parse(
    await readFile(new URL('../../../fixtures/reference-book.json', import.meta.url), 'utf8')
  ) as ReferenceBook;
}

/** One API call with any method and an exact expected status. */
async function call<T>(page: Page, method: string, path: string, body: unknown, status: number, why: string): Promise<T> {
  const csrfToken = method === 'GET' ? undefined : await csrfTokenFor(page);
  const response = await page.evaluate(
    async ({ method, path, csrfToken, body }) => {
      const headers: Record<string, string> = {};
      if (csrfToken) headers['X-CSRF-Token'] = csrfToken;
      if (body !== undefined) headers['Content-Type'] = 'application/json';
      const res = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
      return { status: res.status, text: await res.text() };
    },
    { method, path, csrfToken, body }
  );
  if (response.status !== status) {
    throw new Error(`${method} ${path} (${why}) returned ${response.status}, expected ${status}: ${response.text}`);
  }
  return (response.text === '' ? undefined : JSON.parse(response.text)) as T;
}

/**
 * Seeds the reference book into the signed-in browser's book. EUR and USD are
 * added when missing; system accounts come from the ledger setup. The shared
 * e2e database needs a per-run suffix; a disposable capture book can pass `''`
 * so its screenshots show the fixture's own names.
 */
export async function seedReferenceBook(page: Page, runSuffix?: string): Promise<SeededReferenceBook> {
  const book = await loadReferenceBook();
  const { csrfToken } = await readyForLedger(page);
  const suffix = runSuffix ?? `X${Date.now().toString(36).toUpperCase()}`;
  const ids = new Map<string, number>();
  const transactions: SeededReferenceBook['transactions'] = { ordinary: [], investment: [] };
  ids.set('EUR', await ensureCurrency(page, csrfToken, 'EUR', 'Euro'));
  ids.set('USD', await ensureCurrency(page, await csrfTokenFor(page), 'USD', 'US Dollar'));
  const balances = await apiJSON<{ accounts: Array<{ account_id: number; system_role?: string }> }>(
    page, 'GET', '/api/v1/ledger/account-balances?include_system=true'
  );
  for (const account of balances.accounts) {
    if (account.system_role) ids.set(`system.${account.system_role}`, account.account_id);
  }

  const id = (key: string): number => {
    const value = ids.get(key.replace(/^@/, ''));
    if (value === undefined) throw new Error(`reference book names unknown key ${key}`);
    return value;
  };
  const resolve = (value: unknown): unknown => {
    if (Array.isArray(value)) return value.map(resolve);
    if (value && typeof value === 'object') {
      return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, resolve(item)]));
    }
    if (typeof value === 'string') return value.startsWith('@') ? id(value) : value.replaceAll('{suffix}', suffix);
    return value;
  };

  for (const step of book.steps) {
    if (step.reconcile) {
      const spec = step.reconcile;
      const session = await call<{ id: number; candidates: Array<{ posting_id: number; entry_date: string }> }>(
        page, 'POST', `/api/v1/accounts/${id(spec.account)}/reconciliations/start`, {
          commodity_id: id(spec.commodity), source_kind: 'statement', statement_date: spec.statement_date,
          statement_balance_value: spec.statement_balance_value, statement_balance_scale: spec.statement_balance_scale
        }, 201, step.why
      );
      const selected = session.candidates.filter((c) => c.entry_date <= spec.statement_date).map((c) => c.posting_id);
      await call(page, 'PATCH', `/api/v1/reconciliations/${session.id}`, { posting_version_ids: selected, change_reason: 'reference book' }, 200, step.why);
      await call(page, 'POST', `/api/v1/reconciliations/${session.id}/finish`, { change_reason: 'reference book' }, 200, step.why);
      continue;
    }
    const path = step.path!.split('/').map((segment) => (segment.startsWith('@') ? String(id(segment)) : segment)).join('/');
    const response = await call<Record<string, unknown>>(page, step.method!, path, resolve(step.body), step.status!, step.why);
    if (path === '/api/v1/transactions' && step.method === 'POST') {
      transactions.ordinary.push(response as CreatedTransaction);
    } else if (path.startsWith('/api/v1/investments/')) {
      // Dividend writes return the transaction directly; trades, transfers,
      // splits and capital returns wrap it. Instrument/holding writes do neither.
      const transaction = response.transaction ?? (typeof response.transaction_date === 'string' ? response : undefined);
      if (transaction) transactions.investment.push(transaction as CreatedTransaction);
    }
    for (const [key, field] of Object.entries(step.capture ?? {})) {
      const value = field.split('.').reduce<unknown>((object, name) => (object as Record<string, unknown>)?.[name], response);
      if (typeof value !== 'number') throw new Error(`${step.why}: ${field} is ${String(value)}, not a number`);
      ids.set(key, value);
    }
  }
  return { book, ids, suffix, transactions };
}
