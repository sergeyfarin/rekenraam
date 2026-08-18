const ANNOUNCER_ID = 'id="svelte-announcer"';
const ANNOUNCER_TAG = /<div\s+id="svelte-announcer"[^>]*>/;
const STYLE_ATTRIBUTE = /\s+style="[^"]*"/;

/**
 * Strips the `style` attribute SvelteKit hardcodes on its `#svelte-announcer`
 * live region in the generated root component.
 *
 * The app is served under `style-src 'self'` (see `withSecurityHeaders` in
 * `backend/internal/api/middleware.go`), which also governs `style-src-attr`.
 * Svelte builds that markup through `template.innerHTML`, so parsing it raises
 * a `style-src-attr` violation on every single page load — noise that buries
 * real violations. The announcer's visually-hidden rules live in `app.css`
 * instead, which keeps the CSP free of `'unsafe-inline'` and `'unsafe-hashes'`.
 *
 * Both guards below exist so that a SvelteKit upgrade which moves or rewrites
 * the announcer fails the build instead of silently reintroducing the
 * violation.
 *
 * @returns {import('vite').Plugin}
 */
export function svelteAnnouncerCsp() {
  let seenAnnouncer = false;

  return {
    name: 'rekenraam:svelte-announcer-csp',
    enforce: 'pre',

    transform(code, id) {
      if (!id.endsWith('.svelte') || !code.includes(ANNOUNCER_ID)) {
        return null;
      }

      seenAnnouncer = true;

      const tag = code.match(ANNOUNCER_TAG);
      if (!tag) {
        throw new Error(
          `${id}: found #svelte-announcer but could not parse its opening tag. ` +
            'Re-check how SvelteKit renders the announcer and update ' +
            'frontend/vite/svelte-announcer-csp.js.'
        );
      }

      const stripped = tag[0].replace(STYLE_ATTRIBUTE, '');
      if (stripped === tag[0]) {
        // Upstream no longer inlines the styles; nothing left to strip.
        return null;
      }

      return { code: code.replace(tag[0], stripped), map: null };
    },

    buildEnd(error) {
      if (error || seenAnnouncer) {
        return;
      }

      throw new Error(
        'rekenraam:svelte-announcer-csp never saw #svelte-announcer. SvelteKit ' +
          'likely renamed or removed it; confirm no inline style attribute is ' +
          'left in the generated root component, then update or drop ' +
          'frontend/vite/svelte-announcer-csp.js.'
      );
    }
  };
}
