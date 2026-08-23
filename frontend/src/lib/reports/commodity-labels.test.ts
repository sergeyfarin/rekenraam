import { describe, expect, it } from 'vitest';
import { commodityLabelMap } from './commodity-labels';

describe('commodityLabelMap', () => {
  it('names currencies by their code', () => {
    const labels = commodityLabelMap([{ id: 1, code: 'USD' }], []);
    expect(labels.get(1)).toBe('USD');
  });

  it('names an instrument commodity, so a filterable commodity is also readable', () => {
    const labels = commodityLabelMap([{ id: 1, code: 'USD' }], [
      { commodity_id: 7, commodity_code: 'AAPL', display_name: 'Apple Inc.' }
    ]);
    expect(labels.get(7)).toBe('AAPL');
  });

  it('falls back to the display name when an instrument has no commodity code', () => {
    const labels = commodityLabelMap([], [
      { commodity_id: 7, commodity_code: '', display_name: 'Apple Inc.' }
    ]);
    expect(labels.get(7)).toBe('Apple Inc.');
  });

  it('lets a currency win over an instrument on the same commodity', () => {
    const labels = commodityLabelMap([{ id: 7, code: 'USD' }], [
      { commodity_id: 7, commodity_code: 'AAPL', display_name: 'Apple Inc.' }
    ]);
    expect(labels.get(7)).toBe('USD');
  });

  it('skips an instrument with nothing to call it rather than naming it empty', () => {
    const labels = commodityLabelMap([], [{ commodity_id: 7, commodity_code: null, display_name: null }]);
    expect(labels.has(7)).toBe(false);
  });

  it('handles absent data without inventing entries', () => {
    expect(commodityLabelMap(undefined, undefined).size).toBe(0);
  });
});
