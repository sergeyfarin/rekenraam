import { describe, expect, it } from 'vitest';
import { requiresSecureAuthenticationOrigin } from './auth-origin';

describe('requiresSecureAuthenticationOrigin', () => {
  it('identifies plain HTTP LAN origins that cannot retain secure auth cookies', () => {
    expect(requiresSecureAuthenticationOrigin({ protocol: 'http:', hostname: '10.81.1.177' })).toBe(true);
    expect(requiresSecureAuthenticationOrigin({ protocol: 'http:', hostname: 'rekenraam.lan' })).toBe(true);
  });

  it('allows HTTPS and browser-trusted localhost development origins', () => {
    expect(requiresSecureAuthenticationOrigin({ protocol: 'https:', hostname: '10.81.1.177' })).toBe(false);
    expect(requiresSecureAuthenticationOrigin({ protocol: 'http:', hostname: 'localhost' })).toBe(false);
    expect(requiresSecureAuthenticationOrigin({ protocol: 'http:', hostname: 'app.localhost' })).toBe(false);
    expect(requiresSecureAuthenticationOrigin({ protocol: 'http:', hostname: '127.0.0.1' })).toBe(false);
    expect(requiresSecureAuthenticationOrigin({ protocol: 'http:', hostname: '[::1]' })).toBe(false);
  });
});
