import { describe, expect, it } from 'vitest';
import { EntryContext } from '#lib/transactions/entry-context.svelte.ts';

const access = { date: () => '', setDate() {}, memo: () => '', setMemo() {}, reference: () => '', setReference() {} };

describe('entry request keys (#660)', () => {
  it('keeps one key per activity for as long as the entry is open', () => {
    const entry = new EntryContext(access);
    const buy = entry.requestKey('buy');
    // Closing Buy and opening it again inside the entry finds the same key,
    // with any save whose outcome is unknown.
    expect(entry.requestKey('buy')).toBe(buy);
    expect(entry.requestKey('sell')).not.toBe(buy);
    // A new entry is a new record.
    expect(new EntryContext(access).requestKey('buy').value).not.toBe(buy.value);
  });
});
