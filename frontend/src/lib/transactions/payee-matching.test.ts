import { describe, expect, it } from 'vitest';
import type { PayeeResponse } from '#lib/api/payees.ts';
import {
  exactPayeeMatch,
  normalizePayeeName,
  payeeLookupRequest,
  payeeSuggestions,
  resolvePayeeSave
} from './payee-matching';

const payee = (id: number, name: string): PayeeResponse => ({ id, name }) as unknown as PayeeResponse;

const payees = [
  payee(1, 'Market Hall'),
  payee(2, 'Corner Bakery'),
  payee(3, 'City Transit Authority'),
  payee(4, 'Dr. Wong Dental')
];

describe('normalizePayeeName', () => {
  it('matches how payee records are stored: trimmed, collapsed, lowercased', () => {
    expect(normalizePayeeName('  Market   Hall ')).toBe('market hall');
  });
});

describe('exactPayeeMatch', () => {
  it('matches through casing and spacing', () => {
    expect(exactPayeeMatch('  market   HALL ', payees)?.id).toBe(1);
  });

  it('does not match a different name', () => {
    expect(exactPayeeMatch('Markt Hall', payees)).toBeUndefined();
  });

  it('treats an empty name as no match rather than matching anything', () => {
    expect(exactPayeeMatch('   ', payees)).toBeUndefined();
  });
});

describe('payeeSuggestions', () => {
  it('finds a payee through a typo, which is the whole point', () => {
    expect(payeeSuggestions('Markt Hall', payees).map((p) => p.id)).toContain(1);
  });

  it('finds a payee through a partial name', () => {
    expect(payeeSuggestions('bakery', payees).map((p) => p.id)).toContain(2);
  });

  it('finds a payee when the words are in another order', () => {
    expect(payeeSuggestions('Hall Market', payees).map((p) => p.id)).toContain(1);
  });

  it('returns nothing for an empty query or an empty book', () => {
    expect(payeeSuggestions('', payees)).toEqual([]);
    expect(payeeSuggestions('Market', [])).toEqual([]);
  });
});

describe('transaction editor payee lookup', () => {
  const openingName = 'Old unlinked payee';

  it('keeps a save available for an untouched unlinked name', () => {
    const request = payeeLookupRequest(openingName, openingName, 'untouched');
    expect(request.term).toBe('');
    expect(resolvePayeeSave(request, 'pending', undefined).kind).toBe('save');
  });

  it.each(['focused', 'typed'] as const)('looks up the opening name when %s without gating the save', (interaction) => {
    const request = payeeLookupRequest(` ${openingName} `, openingName, interaction);
    expect(request.term).toBe(openingName);
    expect(resolvePayeeSave(request, 'pending', undefined).kind).toBe('save');
    expect(resolvePayeeSave(request, 'failed', undefined).kind).toBe('save');
    expect(resolvePayeeSave(request, 'ready', undefined).kind).toBe('save');
  });

  it('lets the owner clear the payee without a lookup', () => {
    const request = payeeLookupRequest(' ', openingName, 'typed');
    expect(request.term).toBe('');
    expect(resolvePayeeSave(request, 'pending', undefined).kind).toBe('save');
  });

  it('requires a successful lookup before linking or confirming a changed name', () => {
    const request = payeeLookupRequest('New payee', openingName, 'typed');
    expect(request.term).toBe('New payee');
    const exact = { id: 5, name: 'New payee', exact: true };
    expect(resolvePayeeSave(request, 'pending', undefined).kind).toBe('block');
    expect(resolvePayeeSave(request, 'failed', undefined).kind).toBe('block');
    expect(resolvePayeeSave(request, 'ready', undefined).kind).toBe('confirm');
    expect(resolvePayeeSave(request, 'ready', exact)).toEqual({ kind: 'link', payee: exact });
  });
});
