import { describe, expect, it } from 'vitest';
import { changesCategories } from './categories.ts';

describe('category writes the shell invalidates after (#664 review)', () => {
  it.each([
    '/api/v1/categories',
    '/api/v1/categories/12',
    '/api/v1/categories/12/disable',
    '/api/v1/categories/12/restore',
    '/api/v1/setup/categories'
  ])('%s changes category lists', (path) => {
    expect(changesCategories(path)).toBe(true);
  });

  it.each(['/api/v1/categoriesx', '/api/v1/accounts/12/archive', '/api/v1/tags/3/archive', '/api/v1/setup/system-accounts'])(
    '%s does not',
    (path) => {
      expect(changesCategories(path)).toBe(false);
    }
  );
});
