export type BrowserOrigin = Pick<Location, 'protocol' | 'hostname'>;

const loopbackHostnames = new Set(['localhost', '127.0.0.1', '::1', '[::1]']);

/**
 * Secure authentication cookies cannot be retained over plain HTTP on a LAN
 * hostname or address. Browsers make an exception for localhost development.
 */
export function requiresSecureAuthenticationOrigin(origin: BrowserOrigin): boolean {
  if (origin.protocol === 'https:') return false;

  const hostname = origin.hostname.toLowerCase().replace(/\.$/, '');
  return !loopbackHostnames.has(hostname) && !hostname.endsWith('.localhost');
}
