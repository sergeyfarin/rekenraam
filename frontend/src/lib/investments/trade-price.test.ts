import { describe, expect, it } from 'vitest';
import { newTradeCharge, type TradeChargeDraft } from './trade-economics';
import { pricedTrade } from './trade-price';

const base = { exactMode: false, charges: [] as TradeChargeDraft[], cashCommodityID: 1, scale: 2 };
const charge = (kind: TradeChargeDraft['kind'], amount: string, extra: Partial<TradeChargeDraft> = {}) =>
  ({ ...newTradeCharge(1), kind, amount, ...extra });

describe('pricedTrade (#508)', () => {
  it('derives the gross as quantity × unit price at the currency scale', () => {
    expect(pricedTrade({ ...base, side: 'buy', quantity: '10', unitPrice: '12.5' }))
      .toEqual({ ok: true, gross: '125.00', net: '125.00' });
  });

  it('applies each side’s own arithmetic to the same charges', () => {
    const charges = [charge('commission', '2.50'), charge('rebate', '0.50')];
    const priced = { ...base, exactMode: true, charges, quantity: '10', unitPrice: '10' };
    expect(pricedTrade({ ...priced, side: 'buy' })).toMatchObject({ gross: '100.00', net: '102.00' });
    expect(pricedTrade({ ...priced, side: 'sell' })).toMatchObject({ gross: '100.00', net: '98.00' });
  });

  it('leaves out charges paid separately or in another currency, like the backend', () => {
    const charges = [charge('commission', '2', { separatePayment: true }), charge('transaction_tax', '3', { commodityID: '9' })];
    expect(pricedTrade({ ...base, exactMode: true, charges, side: 'buy', quantity: '1', unitPrice: '100' }))
      .toMatchObject({ net: '100.00' });
  });

  it('ignores charges outside exact mode, where the total is the gross', () => {
    expect(pricedTrade({ ...base, charges: [charge('commission', '5')], side: 'buy', quantity: '2', unitPrice: '3' }))
      .toMatchObject({ net: '6.00' });
  });

  it('never rounds: a product finer than the currency is refused with its exact value', () => {
    expect(pricedTrade({ ...base, side: 'sell', quantity: '1.5', unitPrice: '10.25' }))
      .toEqual({ ok: false, reason: 'precision', exact: '15.375' });
    // Trailing zeros beyond the currency scale are not precision.
    expect(pricedTrade({ ...base, side: 'sell', quantity: '1.50', unitPrice: '10.200' })).toMatchObject({ ok: true, gross: '15.30' });
  });

  it('refuses missing, zero, negative or unparseable inputs and invalid charges', () => {
    for (const [quantity, unitPrice] of [['', '1'], ['1', ''], ['0', '1'], ['-1', '1'], ['x', '1']]) {
      expect(pricedTrade({ ...base, side: 'buy', quantity, unitPrice })).toMatchObject({ ok: false, reason: 'invalid' });
    }
    expect(pricedTrade({ ...base, exactMode: true, charges: [charge('commission', '0.001')], side: 'buy', quantity: '1', unitPrice: '1' }))
      .toMatchObject({ ok: false, reason: 'charges' });
  });

  it('refuses a sale whose charges consume the proceeds', () => {
    expect(pricedTrade({ ...base, exactMode: true, charges: [charge('commission', '10')], side: 'sell', quantity: '1', unitPrice: '10' }))
      .toMatchObject({ ok: false, reason: 'non_positive' });
  });
});
