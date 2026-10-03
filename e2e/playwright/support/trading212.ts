/**
 * Trading 212 provider stub control for e2e runs (T-128).
 *
 * The stub (stubs/trading212.mjs) runs beside the app; the app reaches it
 * through the development-only TRADING212_BASE_URL. Each spec registers its
 * own API key, so accounts never leak between specs sharing one database.
 */

import { test } from '@playwright/test';
import process from 'node:process';

const stubURL = process.env.E2E_TRADING212_STUB_URL ?? `http://127.0.0.1:${process.env.TRADING212_STUB_PORT ?? '16890'}`;

/** One executed order fill, in the values the provider reports. */
export type StubFill = {
  id: number;
  side: 'BUY' | 'SELL';
  quantity: string;
  price: string;
  /** Signed net cash effect: negative for a buy, positive for a sale. */
  netValue: string;
  /** `YYYY-MM-DD`; the stub reports it at 10:00 UTC. */
  date: string;
};

export type StubInstrument = { ticker: string; isin: string; currency: string };

/**
 * Skips the calling spec when the app under test is external (E2E_BASE_URL)
 * and no stub was named for it: an external app cannot be assumed to point
 * at this stub.
 */
export function requireTrading212Stub(): void {
  test.skip(Boolean(process.env.E2E_BASE_URL) && !process.env.E2E_TRADING212_STUB_URL,
    'needs the Trading 212 stub; set E2E_TRADING212_STUB_URL for an external app');
}

/** Replaces the account's whole order history with these fills. */
export async function setTrading212Fills(apiKey: string, instrument: StubInstrument, fills: StubFill[]): Promise<void> {
  const orders = fills.map((fill) => ({
    id: fill.id, side: fill.side, quantity: fill.quantity, price: fill.price, netValue: fill.netValue,
    filledAt: `${fill.date}T10:00:00Z`, ...instrument
  }));
  const res = await fetch(`${stubURL}/__control/accounts/${encodeURIComponent(apiKey)}`, {
    method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ orders })
  });
  if (res.status !== 204) throw new Error(`trading212 stub control failed with ${res.status}: ${await res.text()}`);
}
