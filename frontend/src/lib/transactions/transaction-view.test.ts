import { describe, expect, it } from 'vitest';
import { parseTransactionView, transactionViewHref } from './transaction-view';

function params(query = ''): URLSearchParams {
  return new URLSearchParams(query);
}

describe('transaction views', () => {
  it('defaults to All and ignores an unknown view', () => {
    expect(parseTransactionView(params())).toBe('all');
    expect(parseTransactionView(params('view=unknown'))).toBe('all');
    expect(parseTransactionView(params('view=drafts'))).toBe('drafts');
    expect(parseTransactionView(params('view=deleted'))).toBe('deleted');
  });

  it('keeps every All filter, including an explicit status, in its link', () => {
    const current = params('status=voided&account_id=4&after_date=2026-01-01&q=rent');
    expect(transactionViewHref('all', current)).toBe(
      '/app/ledger/transactions?status=voided&account_id=4&after_date=2026-01-01&q=rent'
    );
  });

  it('keeps ordinary filters in Drafts but lets the view own status', () => {
    const current = params('view=drafts&status=draft&account_id=4&after_date=2026-01-01&q=rent');
    expect(transactionViewHref('drafts', current)).toBe(
      '/app/ledger/transactions?account_id=4&after_date=2026-01-01&q=rent&view=drafts'
    );
  });

  it('does not carry ordinary filters into Deleted', () => {
    expect(transactionViewHref('deleted', params('account_id=4&q=rent'))).toBe(
      '/app/ledger/transactions?view=deleted'
    );
  });
});
