import { describe, expect, it } from 'vitest';
import type { PayeeResponse } from '#lib/api/payees.ts';
import {
  exactPayeeMatch,
  normalizePayeeName,
  payeeSuggestions
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
