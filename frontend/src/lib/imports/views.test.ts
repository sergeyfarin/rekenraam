import { describe, expect, it } from 'vitest';
import { importView, importViewURL } from './views.ts';
describe('Import views (#447)', () => {
  it('reads direct, historical connection and unknown views', () => {
    for (const view of ['files', 'connections', 'rules', 'dividends']) expect(importView(new URLSearchParams({ view }))).toBe(view);
    expect(importView(new URLSearchParams('tab=connections'))).toBe('connections');
    expect(importView(new URLSearchParams('view=bogus'))).toBe('files');
  });
  it('keeps batch, filters, duplicate query parameters and the fragment', () => {
    const url = new URL('https://example.test/app/ledger/import?tab=connections&batch_id=42&filter=a&filter=b#review');
    expect(importViewURL(url, 'dividends')).toBe('/app/ledger/import?batch_id=42&filter=a&filter=b&view=dividends#review');
  });
});
