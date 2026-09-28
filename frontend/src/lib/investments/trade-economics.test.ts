import { describe, expect, it } from 'vitest';
import { correctionTradeDraft, exactTradeFields, newTradeCharge } from './trade-economics';
import type { InvestmentTradeCorrectionContextResponse } from '$lib/api/investments';

it('prefills a correction from exact signed source amounts and recorded fee treatment', () => {
  const source: InvestmentTradeCorrectionContextResponse = {
    operation_id: 1, transaction_id: 2, operation_kind: 'buy', event_date: '2026-01-01',
    holding_account_id: 3, commodity_id: 4, commodity_code: 'ABC', cost_commodity_id: 5,
    quantity_value: '123456789012345678901', quantity_scale: 8, cost_basis_method: '',
    cash_account_id: 6, net_value: '-10200', net_scale: 2, settlement_date: '2026-01-03',
    gross_value: '-10000', gross_scale: 2, memo: '', imported: false,
    already_corrected: false, elected_lots: [], effective_elected_lots: [],
    available_lots: [], can_replace_sale: false,
    charges: [{ kind: 'commission', amount_value: '-200', amount_scale: 2,
      commodity_id: 5, treatment: 'clearing_included', paid_on: '2026-01-04',
      cash_account_id: 7 }]
  };
  const draft = correctionTradeDraft(source);
  expect(draft.quantity).toBe('1234567890123.45678901');
  expect(draft.net).toBe('102.00');
  expect(draft.gross).toBe('100.00');
  expect(draft.charges).toMatchObject([{ amount: '2.00', treatment: 'clearing_included',
    separatePayment: true, cashAccountID: '7' }]);
});

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
