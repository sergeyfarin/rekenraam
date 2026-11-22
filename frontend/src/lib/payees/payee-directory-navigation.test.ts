import { describe, expect, it } from 'vitest';
import { changePayeeDirectorySelection, payeeDirectoryURL, readPayeeDirectorySelection } from './payee-directory-navigation.ts';

const url = new URL('http://localhost/app/ledger/payees?highlight=kept&status=archived&q=shop#directory');
const first = readPayeeDirectorySelection(url.searchParams);
const second = changePayeeDirectorySelection(first, { kind: 'next', cursor: 'second' });
const third = changePayeeDirectorySelection(second, { kind: 'next', cursor: 'third' });

describe('Payees directory navigation', () => {
  it('keeps search, status and cursor in the URL and restores earlier cursors from history', () => {
    const location = payeeDirectoryURL(url.href, third);
    expect(location.searchParams.get('status')).toBe('archived');
    expect(location.searchParams.get('q')).toBe('shop');
    expect(location.searchParams.get('cursor')).toBe('third');
    expect(location.searchParams.get('highlight')).toBe('kept');
    expect(location.hash).toBe('#directory');
    const restored = readPayeeDirectorySelection(location.searchParams, third);
    expect(changePayeeDirectorySelection(restored, { kind: 'previous' })).toEqual(second);
    expect(changePayeeDirectorySelection(second, { kind: 'previous' })).toEqual(first);
  });

  it.each([
    [{ kind: 'filter', q: '  cafe  ' }, 'archived', 'cafe'],
    [{ kind: 'filter', status: 'all' }, 'all', 'shop']
  ] as const)('starts at page one when the search or status changes (%j)', (change, status, q) => {
    const filtered = changePayeeDirectorySelection(third, change);
    expect(filtered).toEqual({ status, q, previousCursors: [] });
    expect(payeeDirectoryURL(payeeDirectoryURL(url.href, third).href, filtered).searchParams.has('cursor')).toBe(false);
  });

  it('keeps the current page when the search only changes whitespace', () => {
    expect(changePayeeDirectorySelection(third, { kind: 'filter', q: ' shop ' })).toEqual(third);
  });

  it('opens a shared cursor and returns to page one when no matching history exists', () => {
    const shared = readPayeeDirectorySelection(new URLSearchParams('status=invalid&q=other&cursor=third'), third);
    expect(shared).toEqual({ status: 'active', q: 'other', cursor: 'third', previousCursors: [] });
    expect(changePayeeDirectorySelection(shared, { kind: 'previous' })).toMatchObject({ q: 'other', cursor: undefined });
  });
});
