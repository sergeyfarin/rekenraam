import { describe, expect, it } from 'vitest';
import { receivingHolding } from './receiving-holding.ts';

const accounts = [
  { id: 1, name: 'Old A', default_commodity_id: 10 },
  { id: 2, name: 'Old B', default_commodity_id: 10 },
  { id: 3, name: 'New A', default_commodity_id: 20 },
  { id: 4, name: 'New B', default_commodity_id: 20 },
  { id: 5, name: 'Untied', default_commodity_id: null }
];

describe('receivingHolding', () => {
  it('offers the accounts that can hold the new instrument and suggests one tied to it', () => {
    const result = receivingHolding(accounts, 1, 20, null);
    expect(result.options.map((option) => option.id)).toEqual([3, 4, 5]);
    expect(result.value).toBe('3');
    expect(result.unavailable).toBe(false);
    expect(result.destinationID).toBe(3);
  });

  it('suggests an untied source and sends no destination for it', () => {
    const result = receivingHolding(accounts, 5, 20, null);
    expect(result.options[0]).toEqual({ id: 5, name: 'Untied', source: true });
    expect(result.value).toBe('5');
    expect(result.destinationID).toBeUndefined();
  });

  it('keeps a chosen account when the source changes', () => {
    expect(receivingHolding(accounts, 1, 20, '4').value).toBe('4');
    const moved = receivingHolding(accounts, 2, 20, '4');
    expect(moved.value).toBe('4');
    expect(moved.unavailable).toBe(false);
  });

  it('keeps a chosen source account identified after the source changes', () => {
    const moved = receivingHolding([...accounts, { id: 6, name: 'Untied 2', default_commodity_id: null }], 6, 20, '5');
    expect(moved.value).toBe('5');
    expect(moved.destinationID).toBe(5);
  });

  it('reports a choice that cannot hold the new instrument instead of replacing it', () => {
    const result = receivingHolding(accounts, 1, 30, '4');
    expect(result.value).toBe('4');
    expect(result.unavailable).toBe(true);
  });

  it('reports a chosen account that is not listed on the date', () => {
    expect(receivingHolding(accounts, 1, 20, '99').unavailable).toBe(true);
  });

  it('has no options without a source or new instrument', () => {
    expect(receivingHolding(accounts, undefined, 20, null).options).toEqual([]);
    expect(receivingHolding(accounts, 1, 0, null).value).toBe('');
  });
});

