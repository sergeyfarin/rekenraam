import { describe, expect, it } from 'vitest';
import { parseTransactionFilters, writeTransactionFilters } from './transaction-url-filters';

const params = (query: string) => new URLSearchParams(query);

describe('parseTransactionFilters', () => {
  it('reads a drill-down link', () => {
    const filters = parseTransactionFilters(
      params('after_date=2026-06-01&before_date=2026-06-30&category_id=7&date_basis=entry')
    );
    expect(filters.afterDate).toBe('2026-06-01');
    expect(filters.beforeDate).toBe('2026-06-30');
    expect(filters.categoryID).toBe(7);
    expect(filters.dateBasis).toBe('entry');
  });

  it('drops values the API would reject instead of forwarding them', () => {
    const filters = parseTransactionFilters(
      params('status=sideways&kind=nonsense&category_type=asset&date_basis=guess&account_id=0&after_date=06-2026')
    );
    expect(filters.status).toBeUndefined();
    expect(filters.kind).toBeUndefined();
    expect(filters.categoryType).toBeUndefined();
    expect(filters.dateBasis).toBeUndefined();
    expect(filters.accountID).toBeUndefined();
    expect(filters.afterDate).toBeUndefined();
  });

  it('drops an inverted range rather than sending a 400', () => {
    const filters = parseTransactionFilters(params('after_date=2026-06-30&before_date=2026-06-01'));
    expect(filters.afterDate).toBeUndefined();
    expect(filters.beforeDate).toBeUndefined();
  });

  it('keeps an equal-day range, which is a valid single day', () => {
    const filters = parseTransactionFilters(params('after_date=2026-06-01&before_date=2026-06-01'));
    expect(filters.afterDate).toBe('2026-06-01');
    expect(filters.beforeDate).toBe('2026-06-01');
  });

  it('treats needs_review as true only for the literal true', () => {
    expect(parseTransactionFilters(params('needs_review=true')).needsReview).toBe(true);
    expect(parseTransactionFilters(params('needs_review=1')).needsReview).toBeUndefined();
  });
});

describe('writeTransactionFilters', () => {
  it('round-trips a parsed link', () => {
    const query = 'after_date=2026-06-01&before_date=2026-06-30&category_id=7&date_basis=entry';
    const filters = parseTransactionFilters(params(query));
    expect(parseTransactionFilters(writeTransactionFilters(params(''), filters))).toEqual(filters);
  });

  it('removes a cleared filter from the URL', () => {
    const next = writeTransactionFilters(params('account_id=4&q=rent'), { accountID: 4 });
    expect(next.get('account_id')).toBe('4');
    expect(next.has('q')).toBe(false);
  });

  it('preserves parameters it does not own', () => {
    const next = writeTransactionFilters(params('highlight=12'), { payeeID: 3 });
    expect(next.get('highlight')).toBe('12');
    expect(next.get('payee_id')).toBe('3');
  });

  it('omits needs_review when it is off, rather than writing false', () => {
    expect(writeTransactionFilters(params(''), { needsReview: false }).has('needs_review')).toBe(false);
  });
});
