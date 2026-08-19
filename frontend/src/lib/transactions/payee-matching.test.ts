import { describe, expect, it } from 'vitest';
import type { PayeeResponse } from '$lib/api/payees';
import {
  exactPayeeMatch,
  needsPayeeConfirmation,
  normalizePayeeName,
  payeeSuggestions,
  resolveTypedPayee
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

describe('resolveTypedPayee', () => {
  it('is none when nothing was typed', () => {
    expect(resolveTypedPayee('', undefined, payees).kind).toBe('none');
    expect(resolveTypedPayee('   ', undefined, payees).kind).toBe('none');
  });

  it('is linked when a record already carries the name', () => {
    const resolution = resolveTypedPayee('market hall', undefined, payees);
    expect(resolution).toMatchObject({ kind: 'linked' });
    expect(resolution.kind === 'linked' && resolution.payee.id).toBe(1);
  });

  it('is linked when the user picked from the list, whatever the text says', () => {
    const resolution = resolveTypedPayee('anything', 2, payees);
    expect(resolution.kind === 'linked' && resolution.payee.id).toBe(2);
  });

  it('is unknown with suggestions when no record carries the name', () => {
    const resolution = resolveTypedPayee('Markt Hall', undefined, payees);
    expect(resolution.kind).toBe('unknown');
    expect(resolution.kind === 'unknown' && resolution.suggestions.map((p) => p.id)).toContain(1);
    // The trimmed name is what a new record would be created as.
    expect(resolution.kind === 'unknown' && resolution.name).toBe('Markt Hall');
  });
});

describe('needsPayeeConfirmation', () => {
  const unknown = resolveTypedPayee('Markt Hall', undefined, payees);

  it('asks when a new name was typed', () => {
    expect(needsPayeeConfirmation(unknown, true)).toBe(true);
  });

  it('stays silent when the user did not touch the payee', () => {
    // Editing an unrelated field on a transaction that predates this rule must
    // not be blocked by history the user did not create.
    expect(needsPayeeConfirmation(unknown, false)).toBe(false);
  });

  it('stays silent for a name that resolves', () => {
    expect(needsPayeeConfirmation(resolveTypedPayee('Market Hall', undefined, payees), true)).toBe(false);
    expect(needsPayeeConfirmation(resolveTypedPayee('', undefined, payees), true)).toBe(false);
  });
});
