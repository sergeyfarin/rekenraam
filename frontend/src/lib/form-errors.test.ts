import { describe, expect, it } from 'vitest';
import { APIClientError } from '#lib/api/client.ts';
import { getFormErrorState } from './form-errors.ts';
import { getLocale, overwriteGetLocale } from '#lib/paraglide/runtime.js';
import { m } from '#lib/paraglide/messages.js';
import { builtinCategoryLabel } from '#lib/categories/category-labels.ts';
import { formatCalendarDate } from '#lib/dates/calendar-date.ts';

describe('reference backdating refusal in category and institution forms', () => {
  it.each(['en', 'es', 'fr', 'nl', 'de', 'ru'] as const)('shows translated guidance and request attribution in %s', (locale) => {
    const previous = getLocale;
    try {
      overwriteGetLocale(() => locale);
      const state = getFormErrorState(new APIClientError({
        status: 409,
        code: 'REFERENCE_VERSION_BACKDATED',
        detail: 'server detail must not replace translated guidance',
        requestId: 'backdating-request'
      }));
      expect(state).toEqual({
        message: m.api_error_reference_version_backdated(),
        requestId: 'backdating-request'
      });
      expect(state?.message).not.toBe(m.api_error_generic());
      expect(state?.message).not.toBe(m.api_error_conflict());
      if (locale !== 'en') {
        expect(state?.message).not.toBe(m.api_error_reference_version_backdated({}, { locale: 'en' }));
      }
    } finally {
      overwriteGetLocale(previous);
    }
  });
});

describe('dated reference conflicts name the rule, day and record (#552)', () => {
  const rules = [
    ['category_name_taken', m.api_error_reference_conflict_category_name_taken],
    ['payee_name_taken', m.api_error_reference_conflict_payee_name_taken],
    ['category_parent_archived', m.api_error_reference_conflict_category_parent_archived],
    ['category_parent_cycle', m.api_error_reference_conflict_category_parent_cycle],
    ['category_active_child', m.api_error_reference_conflict_category_active_child],
    ['institution_linked_account', m.api_error_reference_conflict_institution_linked_account]
  ] as const;

  it.each(['en', 'es', 'fr', 'nl', 'de', 'ru'] as const)('explains each rule with a locale date in %s', (locale) => {
    const previous = getLocale;
    try {
      overwriteGetLocale(() => locale);
      const date = formatCalendarDate('2026-09-01', locale);
      for (const [rule, message] of rules) {
        const state = getFormErrorState(new APIClientError({
          status: 409,
          code: 'REFERENCE_DATED_CONFLICT',
          detail: 'server detail must not replace translated guidance',
          requestId: 'dated-request',
          conflict: { rule, date: '2026-09-01', name: 'Groceries' }
        }));
        expect(state).toEqual({ message: message({ date, name: 'Groceries' }), requestId: 'dated-request' });
        expect(state?.message).toContain(date);
        expect(state?.message).toContain('Groceries');
        if (locale !== 'en') {
          expect(state?.message).not.toBe(message({ date, name: 'Groceries' }, { locale: 'en' }));
        }
      }
    } finally {
      overwriteGetLocale(previous);
    }
  });

  it('labels an unnamed built-in category by its key', () => {
    const state = getFormErrorState(new APIClientError({
      status: 409,
      code: 'REFERENCE_DATED_CONFLICT',
      conflict: { rule: 'category_parent_archived', date: '2026-09-01', builtinKey: 'expense.housing' }
    }));
    expect(state?.message).toContain(builtinCategoryLabel('expense.housing'));
  });

  it('falls back to the code message without details', () => {
    const state = getFormErrorState(new APIClientError({ status: 409, code: 'REFERENCE_DATED_CONFLICT' }));
    expect(state?.message).toBe(m.api_error_reference_dated_conflict());
  });

  it('names the earliest allowed date on a backdating refusal', () => {
    const state = getFormErrorState(new APIClientError({
      status: 409,
      code: 'REFERENCE_VERSION_BACKDATED',
      conflict: { date: '2026-06-01' }
    }));
    expect(state?.message).toBe(m.api_error_reference_version_backdated_from({ date: formatCalendarDate('2026-06-01', getLocale()) }));
  });
});
