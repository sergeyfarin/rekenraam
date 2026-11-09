import { describe, expect, it, vi } from 'vitest';
import { SearchRequestState } from './search-request.svelte.ts';
import type { SearchResult } from './search-model.ts';

const account: SearchResult = { id: 'account:1', group: 'account', title: 'Checking', href: '/app/ledger/accounts/1/register' };
const spending: SearchResult = { id: 'page:/app/reports/spending', group: 'page', title: 'Spending', href: '/app/reports/spending' };

// The component's own sequence: release closes without ending the request,
// act runs the handler or the default action, failed reopens with an error.
function choose(state: SearchRequestState, item: SearchResult, log: { defaults: SearchResult[]; reopened: number }) {
  return state.choose(item, {
    release: () => {},
    act: async (select, chosen) => { if (select) await select(chosen); else log.defaults.push(chosen); },
    failed: () => { log.reopened += 1; }
  });
}

describe('search request ownership (#490)', () => {
  it('applies a scoped handler only while its scope is shown', async () => {
    const state = new SearchRequestState(() => 'all', () => undefined);
    const onSelect = vi.fn();
    state.start({ scope: 'account', onSelect });
    expect(state.scope).toBe('account');

    // Back to All: an ordinary page result navigates as usual.
    state.setScope('all');
    const log = { defaults: [] as SearchResult[], reopened: 0 };
    expect(await choose(state, spending, log)).toBe(true);
    expect(onSelect).not.toHaveBeenCalled();
    expect(log.defaults).toEqual([spending]);
  });

  it('returns to the request handler when its scope is chosen again', async () => {
    const state = new SearchRequestState(() => 'all', () => undefined);
    const onSelect = vi.fn();
    state.start({ scope: 'account', onSelect });
    state.setScope('all');
    state.setScope('account');
    const log = { defaults: [] as SearchResult[], reopened: 0 };
    await choose(state, account, log);
    expect(onSelect).toHaveBeenCalledWith(account);
    // A successful selection ends the request.
    expect(state.request).toBeUndefined();
    expect(state.scope).toBe('all');
  });

  it('keeps a failed request, its scope and handler, for a retry', async () => {
    const state = new SearchRequestState(() => 'all', () => undefined);
    const onSelect = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(undefined);
    state.start({ scope: 'account', onSelect });
    const log = { defaults: [] as SearchResult[], reopened: 0 };
    expect(await choose(state, account, log)).toBe(false);
    expect(log.reopened).toBe(1);
    expect(state.scope).toBe('account');
    expect(state.request?.onSelect).toBe(onSelect);

    expect(await choose(state, account, log)).toBe(true);
    expect(onSelect).toHaveBeenCalledTimes(2);
    expect(log.defaults).toEqual([]);
    expect(state.request).toBeUndefined();
  });

  it('does not restore a failed request once a newer interaction owns search', async () => {
    const state = new SearchRequestState(() => 'all', () => undefined);
    let fail: (reason: Error) => void = () => {};
    const older = vi.fn(() => new Promise<void>((_, reject) => { fail = reject; }));
    state.start({ scope: 'account', onSelect: older });
    const log = { defaults: [] as SearchResult[], reopened: 0 };
    const pending = choose(state, account, log);

    // The owner opens ordinary search (button or shortcut) meanwhile.
    state.dispose();
    state.setScope('instrument');
    fail(new Error('offline'));
    expect(await pending).toBe(false);
    expect(log.reopened).toBe(0);
    expect(state.scope).toBe('instrument');
    expect(state.request).toBeUndefined();

    // Nor when a newer request replaced it.
    const newer = vi.fn();
    state.start({ scope: 'account', onSelect: older });
    const second = choose(state, account, log);
    state.start({ scope: 'category', onSelect: newer });
    fail(new Error('offline'));
    await second;
    expect(log.reopened).toBe(0);
    expect(state.request?.onSelect).toBe(newer);
    expect(state.scope).toBe('category');
  });

  it('reopens ordinary search after a failed default action, and keeps its scope', async () => {
    const state = new SearchRequestState(() => 'all', () => undefined);
    state.setScope('account');
    const failed = vi.fn();
    const ok = await state.choose(account, { release: () => {}, act: () => Promise.reject(new Error('nav')), failed });
    expect(ok).toBe(false);
    expect(failed).toHaveBeenCalledOnce();
    expect(state.scope).toBe('account');
  });

  it('uses the mounted handler and scope without a request, and resets to them', async () => {
    const mounted = vi.fn();
    let base: 'all' | 'account' = 'account';
    const state = new SearchRequestState(() => base, () => mounted);
    expect(state.scope).toBe('account');
    await choose(state, account, { defaults: [], reopened: 0 });
    expect(mounted).toHaveBeenCalledWith(account);

    state.start({ scope: 'instrument', onSelect: vi.fn() });
    base = 'all';
    state.reset();
    expect(state.request).toBeUndefined();
    expect(state.scope).toBe('all');
    // Dismissal ends only a request; ordinary search keeps its chip.
    state.setScope('category');
    state.dispose();
    expect(state.scope).toBe('category');
  });
});
