import { describe, expect, it } from 'vitest';
import { exactTradeFields, newTradeCharge } from './trade-economics';

describe('exactTradeFields', () => {
  it('sends signed gross, net and fee coefficients without a floating point conversion', () => {
    const charge = newTradeCharge(1);
    charge.amount = '2.00';
    const result = exactTradeFields({ side: 'buy', gross: '100.00', netValue: '10200', netScale: 2,
      cashCommodityID: 1, settlementDate: '2026-01-03', charges: [charge] });
    expect(result).toEqual({ ok: true, fields: {
      gross_amount_value: '-10000', gross_amount_scale: 2,
      net_settlement_value: '-10200', net_settlement_scale: 2,
      settlement_date: '2026-01-03',
      charges: [{ kind: 'commission', amount_value: '-200', amount_scale: 2,
        commodity_id: 1, treatment: undefined, charge_account_id: undefined,
        cash_account_id: undefined, paid_on: undefined }]
    } });
  });

  it('keeps a separately paid fee cash account and date', () => {
    const charge = newTradeCharge(1);
    charge.amount = '2';
    charge.separatePayment = true;
    charge.cashAccountID = '7';
    charge.paidOn = '2026-01-04';
    const result = exactTradeFields({ side: 'sell', gross: '120', netValue: '120', netScale: 0,
      cashCommodityID: 1, settlementDate: '', charges: [charge] });
    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.fields.charges?.[0]).toMatchObject({ cash_account_id: 7, paid_on: '2026-01-04' });
    }
  });
});
