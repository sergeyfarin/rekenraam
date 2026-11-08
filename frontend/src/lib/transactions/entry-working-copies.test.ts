import { describe, expect, it } from 'vitest';
import { mainDateRole, tradePair } from '#lib/investments/activities.ts';
import { EntryWorkingCopies, moneyCurrencyStep, restoreTradeCopy } from './entry-working-copies';

describe('EntryWorkingCopies (#508)', () => {
  it('restores what an activity was left with, as an independent copy', () => {
    const copies = new EntryWorkingCopies();
    const charges = [{ amount: '1' }];
    copies.keep('buy', { exactMode: true, charges }, false);
    charges[0].amount = 'changed after keep';
    const restored = copies.restore<{ exactMode: boolean; charges: { amount: string }[] }>('buy')!;
    expect(restored).toEqual({ exactMode: true, charges: [{ amount: '1' }] });
    restored.charges![0].amount = 'changed after restore';
    expect(copies.restore('buy')).toEqual({ exactMode: true, charges: [{ amount: '1' }] });
    expect(copies.restore('sell')).toBeUndefined();
  });

  it('counts only the owner’s changes, never a field that filled itself (#521 review)', () => {
    const copies = new EntryWorkingCopies();
    copies.keep('split', { ratio: '' }, false);
    copies.keep('dividend', { amount: '', incomeAccountID: '' }, false);
    expect(copies.hasUnsavedWork()).toBe(false);
    // A default income account arriving later is not work…
    copies.keep('dividend', { amount: '', incomeAccountID: '4' }, false);
    expect(copies.hasUnsavedWork()).toBe(false);
    // …but the owner choosing another one is, even with nothing else entered.
    copies.keep('dividend', { amount: '', incomeAccountID: '5' }, true);
    expect(copies.hasUnsavedWork()).toBe(true);
    copies.keep('dividend', { amount: '', incomeAccountID: '4' }, true);
    expect(copies.hasUnsavedWork()).toBe(false);
    copies.keep('split', { ratio: '2:1' }, true);
    expect(copies.hasUnsavedWork()).toBe(true);
    copies.keep('split', { ratio: '' }, true);
    expect(copies.hasUnsavedWork()).toBe(false);
  });

  it('counts a shared selection the owner changed, never one a picker filled by itself (#521 review)', () => {
    const copies = new EntryWorkingCopies();
    copies.sharedBaseline = { holdingAccountID: 7 };
    copies.setShared('holdingAccountID', 7, false);
    // A holding's only position filled the currency; the owner did nothing.
    copies.setShared('currencyID', 3, false);
    expect(copies.hasUnsavedWork()).toBe(false);
    copies.setShared('currencyID', 4, true);
    expect(copies.hasUnsavedWork()).toBe(true);
    copies.setShared('currencyID', 3, true);
    expect(copies.hasUnsavedWork()).toBe(false);
    copies.setShared('settlementAccountID', 12, true);
    expect(copies.hasUnsavedWork()).toBe(true);
  });

  it('keeps the owner’s choice owned when a form restores the same value by itself', () => {
    const copies = new EntryWorkingCopies();
    copies.setShared('currencyID', 9, true);
    // Shares out finds the matching position and writes the same currency.
    copies.setShared('currencyID', 9, false);
    expect(copies.hasUnsavedWork()).toBe(true);
    // A different value written automatically becomes the baseline.
    copies.setShared('currencyID', 3, false);
    expect(copies.hasUnsavedWork()).toBe(false);
  });

  it('shares quantity and unit price within a pair, never across pairs', () => {
    const copies = new EntryWorkingCopies();
    copies.setPair('buy', { quantity: '10', unitPrice: '5' });
    expect(copies.pairOf('sell')).toEqual({ quantity: '10', unitPrice: '5' });
    expect(copies.pairOf('short-open')).toBeUndefined();
    expect(copies.pairOf('dividend')).toBeUndefined();
    copies.setPair('dividend', { quantity: '1', unitPrice: '1' });
    expect(copies.pairOf('dividend')).toBeUndefined();
    copies.setPair('short-cover', { quantity: '3', unitPrice: '' });
    expect(copies.pairOf('short-open')).toEqual({ quantity: '3', unitPrice: '' });
    expect(copies.pairOf('buy')).toEqual({ quantity: '10', unitPrice: '5' });
  });
});

describe('restoreTradeCopy (#508)', () => {
  it('starts a new paired activity from the pair, with no total of its own', () => {
    expect(restoreTradeCopy(undefined, { quantity: '10', unitPrice: '5' }))
      .toEqual({ quantity: '10', unitPrice: '5', cashAmount: '', gross: '', recalculate: false });
  });

  it('restores its own total when the pair is unchanged', () => {
    expect(restoreTradeCopy({ quantity: '10', unitPrice: '', cashAmount: '101', gross: '100' }, { quantity: '10', unitPrice: '' }))
      .toEqual({ quantity: '10', unitPrice: '', cashAmount: '101', gross: '100', recalculate: false });
  });

  it('never reuses a total typed for another quantity', () => {
    expect(restoreTradeCopy({ quantity: '10', unitPrice: '', cashAmount: '101', gross: '' }, { quantity: '4', unitPrice: '' }))
      .toEqual({ quantity: '4', unitPrice: '', cashAmount: '', gross: '', recalculate: true });
  });

  it('lets a unit price derive the total again without asking for it', () => {
    expect(restoreTradeCopy({ quantity: '10', unitPrice: '', cashAmount: '101', gross: '' }, { quantity: '4', unitPrice: '25' }))
      .toMatchObject({ quantity: '4', unitPrice: '25', cashAmount: '', recalculate: false });
  });

  it('keeps an untouched form untouched', () => {
    expect(restoreTradeCopy(undefined, undefined)).toEqual({ quantity: '', unitPrice: '', cashAmount: '', gross: '', recalculate: false });
  });
});

describe('activity roles', () => {
  it('names each activity’s main date and its trade pair', () => {
    expect(mainDateRole('sell')).toBe('trade');
    expect(mainDateRole('write-off')).toBe('trade');
    expect(mainDateRole('payment-in-lieu')).toBe('payment');
    expect(mainDateRole('reinvested')).toBe('payment');
    expect(mainDateRole('split')).toBe('effective');
    expect(mainDateRole('internal-transfer')).toBe('effective');
    expect(tradePair('buy')).toBe(tradePair('sell'));
    expect(tradePair('short-open')).toBe(tradePair('short-cover'));
    expect(tradePair('buy')).not.toBe(tradePair('short-open'));
    expect(tradePair('write-off')).toBeUndefined();
  });
});

describe('trade money currency (#521 seventh review)', () => {
  it('drops a total whose pair price was entered in another currency', () => {
    expect(restoreTradeCopy(
      { quantity: '10', unitPrice: '5', cashAmount: '50', gross: '', cashCommodityID: 1 },
      { quantity: '10', unitPrice: '5', currencyID: 2 }))
      .toEqual({ quantity: '10', unitPrice: '5', cashAmount: '', gross: '', cashCommodityID: 2, recalculate: false });
  });

  it('keeps a total and reports its currency when nothing changed', () => {
    expect(restoreTradeCopy(
      { quantity: '10', unitPrice: '', cashAmount: '50', gross: '', cashCommodityID: 1 },
      { quantity: '10', unitPrice: '', currencyID: 1 }).cashCommodityID).toBe(1);
    expect(restoreTradeCopy({ quantity: '10', unitPrice: '', cashAmount: '50', gross: '', cashCommodityID: 1 }, undefined))
      .toMatchObject({ cashAmount: '50', cashCommodityID: 1 });
  });

  it('treats an unknown currency as unchanged', () => {
    expect(restoreTradeCopy(
      { quantity: '10', unitPrice: '', cashAmount: '50', gross: '' },
      { quantity: '10', unitPrice: '', currencyID: 2 }).cashAmount).toBe('50');
  });

  it('clears amounts only when they were typed in a known, different currency', () => {
    expect(moneyCurrencyStep(1, 2, true).clear).toBe(true);
    expect(moneyCurrencyStep(1, 1, true).clear).toBe(false);
    expect(moneyCurrencyStep(undefined, 2, true).clear).toBe(false);
    expect(moneyCurrencyStep(1, 2, false).clear).toBe(false);
  });
});
