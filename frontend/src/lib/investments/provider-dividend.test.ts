import { describe, expect, it } from 'vitest';
import {
  grossPrefill,
  parseProviderDividendEvidence,
  paymentInLieuProposal,
  proposalCommodityID,
  proposalShape,
  providerDividendRevisions
} from './provider-dividend.ts';

describe('provider dividend evidence', () => {
  it('reads schema-1 evidence and keeps unknown figures null', () => {
    const evidence = parseProviderDividendEvidence({
      schema: 1,
      symbol: 'ASML',
      mic: 'XAMS',
      ex_date: '2026-07-29',
      entitlement_date: '2026-07-28',
      amount_per_share: null,
      listing_currency: 'EUR',
      entitled_units: { value: '10', scale: 0 },
      estimated_gross: { value: 'x', scale: 0 },
      missing: ['payment_date', 7, 'amount_per_share']
    });
    expect(evidence).toMatchObject({
      symbol: 'ASML',
      exDate: '2026-07-29',
      amountPerShare: null,
      entitledUnits: { value: '10', scale: 0 },
      estimatedGross: null,
      missing: ['payment_date', 'amount_per_share'],
      unsupported: null
    });
    expect(parseProviderDividendEvidence({ schema: 2 })).toBeNull();
    expect(parseProviderDividendEvidence({})).toBeNull();
  });

  it('classifies proposals by what the owner can do', () => {
    expect(proposalShape({ kind: 'dividend_income', complete: false })).toBe('needs_completion');
    expect(proposalShape({ kind: 'dividend_income', amount_value: 1 })).toBe('complete');
    expect(proposalShape({ kind: 'informational', reason: 'short_position' })).toBe('informational');
    expect(proposalShape({ kind: 'split' })).toBe('unsupported');
    expect(proposalShape({ kind: 'future_kind' })).toBe('unsupported');
    expect(proposalCommodityID({ commodity_id: 4 })).toBe(4);
    expect(proposalCommodityID({ commodity_id: '4' })).toBeUndefined();
  });

  it('reads a short entitlement as a payment in lieu the owner completes (#255)', () => {
    const raw = { kind: 'payment_in_lieu', complete: false, commodity_id: 4, ex_date: '2026-07-29', holding_account_ids: [9, 'x', 11] };
    expect(proposalShape(raw)).toBe('payment_in_lieu');
    expect(paymentInLieuProposal(raw)).toEqual({ commodityID: 4, exDate: '2026-07-29', holdingAccountIDs: [9, 11] });
    expect(paymentInLieuProposal({ ...raw, holding_account_ids: [] })).toBeNull();
    expect(paymentInLieuProposal({ kind: 'dividend_income', complete: false, commodity_id: 4 })).toBeNull();
    expect(proposalShape({ kind: 'informational', reason: 'long_and_short_position' })).toBe('informational');
  });

  it('pre-fills the received gross only in the same currency without rounding', () => {
    expect(grossPrefill({ value: '16', scale: 0 }, 'EUR', 'EUR', 2)).toEqual({ value: '1600', scale: 2 });
    expect(grossPrefill({ value: '16', scale: 0 }, 'EUR', 'USD', 2)).toBeNull();
    expect(grossPrefill({ value: '16001', scale: 3 }, 'EUR', 'EUR', 2)).toBeNull();
    expect(grossPrefill(null, 'EUR', 'EUR', 2)).toBeNull();
  });

  it('chains provider revisions from the first observation in the order seen', () => {
    const first = parseProviderDividendEvidence({ schema: 1, amount_per_share: { value: '16', scale: 1 }, listing_currency: 'EUR' });
    const steps = providerDividendRevisions(first, [
      { evidence: { schema: 1, amount_per_share: { value: '17', scale: 1 }, listing_currency: 'EUR' }, observed_at: '2026-10-01T00:00:00Z' },
      { evidence: { schema: 2 }, observed_at: '2026-10-02T00:00:00Z' },
      { evidence: { schema: 1, amount_per_share: null, listing_currency: 'USD' }, observed_at: '2026-10-03T00:00:00Z' }
    ]);
    expect(steps).toEqual([
      {
        from: { amountPerShare: { value: '16', scale: 1 }, listingCurrency: 'EUR' },
        to: { amountPerShare: { value: '17', scale: 1 }, listingCurrency: 'EUR' },
        observedAt: '2026-10-01T00:00:00Z'
      },
      {
        from: { amountPerShare: { value: '17', scale: 1 }, listingCurrency: 'EUR' },
        to: { amountPerShare: null, listingCurrency: 'USD' },
        observedAt: '2026-10-03T00:00:00Z'
      }
    ]);
    expect(providerDividendRevisions(null, [])).toEqual([]);
    expect(providerDividendRevisions(first, undefined)).toEqual([]);
  });
});
