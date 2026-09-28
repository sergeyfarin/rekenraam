import type { InvestmentTradeRequest } from '$lib/api/investments';
import { parseMoneyMagnitude, type AmountFieldError } from '$lib/investments/form-amounts';
import { formatLedgerAmount } from '$lib/money/amount';
import type { InvestmentTradeCorrectionContextResponse } from '$lib/api/investments';

export type TradeChargeDraft = {
  kind: 'commission' | 'transaction_tax' | 'other_fee' | 'rebate';
  amount: string;
  commodityID: string;
  treatment: '' | 'clearing_included' | 'separately_expensed';
  chargeAccountID: string;
  cashAccountID: string;
  paidOn: string;
  separatePayment: boolean;
};

export function newTradeCharge(commodityID?: number): TradeChargeDraft {
  return { kind: 'commission', amount: '', commodityID: String(commodityID ?? ''), treatment: '',
    chargeAccountID: '', cashAccountID: '', paidOn: '', separatePayment: false };
}

// A correction starts from recorded source facts. Preserve coefficient and
// scale exactly in editable decimal inputs; number conversion would lose
// precision and could silently change a historical trade.
export function correctionTradeDraft(source: InvestmentTradeCorrectionContextResponse) {
  const magnitude = (value: string, scale: number) =>
    formatLedgerAmount(value.startsWith('-') ? value.slice(1) : value, scale);
  return {
    quantity: formatLedgerAmount(source.quantity_value, source.quantity_scale),
    net: magnitude(source.net_value, source.net_scale),
    exactMode: source.gross_value !== undefined,
    gross: source.gross_value !== undefined && source.gross_scale !== undefined
      ? magnitude(source.gross_value, source.gross_scale) : '',
    settlementDate: source.settlement_date,
    charges: source.charges.map((charge): TradeChargeDraft => ({
      kind: charge.kind as TradeChargeDraft['kind'],
      amount: magnitude(charge.amount_value, charge.amount_scale),
      commodityID: String(charge.commodity_id),
      treatment: charge.treatment,
      chargeAccountID: String(charge.charge_account_id ?? ''),
      cashAccountID: String(charge.cash_account_id ?? ''),
      paidOn: charge.paid_on,
      separatePayment: charge.cash_account_id !== undefined
    }))
  };
}

export function exactTradeFields(input: {
  side: 'buy' | 'sell';
  gross: string;
  settlementDate: string;
  charges: TradeChargeDraft[];
  cashCommodityID: number;
  netValue: string;
  netScale: number;
}): { ok: true; fields: Partial<InvestmentTradeRequest> } | { ok: false; reason: AmountFieldError } {
  const gross = parseMoneyMagnitude(input.gross);
  if (!gross.ok) return gross;
  if (gross.field.value === '0') return { ok: false, reason: 'invalid' };
  const signedGross = input.side === 'buy' ? `-${gross.field.value}` : gross.field.value;
  const charges: NonNullable<InvestmentTradeRequest['charges']> = [];
  for (const draft of input.charges) {
    const amount = parseMoneyMagnitude(draft.amount);
    if (!amount.ok) return amount;
    if (amount.field.value === '0') return { ok: false, reason: 'invalid' };
    const signed = draft.kind === 'rebate' ? amount.field.value : `-${amount.field.value}`;
    const chargeCommodityID = Number(draft.commodityID || input.cashCommodityID);
    const separatePayment = draft.separatePayment || chargeCommodityID !== input.cashCommodityID;
    charges.push({
      kind: draft.kind, amount_value: signed, amount_scale: amount.field.scale,
      commodity_id: chargeCommodityID,
      treatment: draft.treatment || undefined,
      charge_account_id: draft.chargeAccountID ? Number(draft.chargeAccountID) : undefined,
      cash_account_id: separatePayment && draft.cashAccountID ? Number(draft.cashAccountID) : undefined,
      paid_on: separatePayment ? (draft.paidOn || undefined) : undefined
    });
  }
  return { ok: true, fields: {
    gross_amount_value: signedGross, gross_amount_scale: gross.field.scale,
    net_settlement_value: input.side === 'buy' ? `-${input.netValue}` : input.netValue,
    net_settlement_scale: input.netScale,
    settlement_date: input.settlementDate || undefined, charges
  } };
}
