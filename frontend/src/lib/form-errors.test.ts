import { describe, expect, it } from 'vitest';
import { APIClientError } from '#lib/api/client.ts';
import { getFormErrorState } from './form-errors.ts';
import { getLocale, overwriteGetLocale } from '#lib/paraglide/runtime.js';
import { m } from '#lib/paraglide/messages.js';

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
