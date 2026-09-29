import { describe, expect, it } from 'vitest';
import { correctionLotDrafts, correctionTradeDraft, exactTradeFields, newTradeCharge,
  parseCorrectionLotChoices } from './trade-economics';
import type { InvestmentTradeCorrectionContextResponse } from '$lib/api/investments';

it('prefills a correction from exact signed source amounts and recorded fee treatment', () => {
  const source: InvestmentTradeCorrectionContextResponse = {
    operation_id: 1, transaction_id: 2, operation_kind: 'buy', event_date: '2026-01-01',
    holding_account_id: 3, commodity_id: 4, commodity_code: 'ABC', cost_commodity_id: 5,
    quantity_value: '123456789012345678901', quantity_scale: 8, cost_basis_method: '',
    cash_account_id: 6, net_value: '-10200', net_scale: 2, settlement_date: '2026-01-03',
    gross_value: '-10000', gross_scale: 2, memo: '', imported: false,
    source_identity_id: 0, source_kind: '',
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

it('uses effective lots and sums fractional correction elections exactly', () => {
  const source = {
    effective_elected_lots: [{ lot_id: 9, quantity_value: '125', quantity_scale: 2 }],
    available_lots: [
      { lot_id: 9, opened_on: '2026-01-01', quantity_value: '2', quantity_scale: 0 },
      { lot_id: 10, opened_on: '2026-01-02', quantity_value: '50', quantity_scale: 2 }
    ]
  } as InvestmentTradeCorrectionContextResponse;
  expect(correctionLotDrafts(source)).toEqual([{ lotID: '9', quantity: '1.25' }]);
  const valid = parseCorrectionLotChoices([{ lotID: '9', quantity: '1.25' },
    { lotID: '10', quantity: '0.5' }], source.available_lots, { value: '175', scale: 2 });
  expect(valid).toEqual({ ok: true, allocations: [
    { lot_id: 9, quantity_value: '125', quantity_scale: 2 },
    { lot_id: 10, quantity_value: '5', quantity_scale: 1 }
  ] });
  expect(parseCorrectionLotChoices([{ lotID: '9', quantity: '2.01' }], source.available_lots,
    { value: '201', scale: 2 })).toEqual({ ok: false, reason: 'exceeds_available' });
  expect(parseCorrectionLotChoices([{ lotID: '9', quantity: '1.25' }], source.available_lots,
    { value: '175', scale: 2 })).toEqual({ ok: false, reason: 'mismatch' });
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
