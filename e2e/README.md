# End-to-End Tests

Browser-level integration tests live here.

## Intended Use

E2E tests should run against the app as users experience it:

1. Start the backend.
2. Serve the frontend through SvelteKit dev server for development checks, or through the Go binary for production checks.
3. Run the browser tests against the chosen base URL.

## Suggested Tooling

Playwright is a good default here because it can test the SvelteKit frontend, backend integration, and final binary behavior from the browser's point of view.

Keep e2e dependencies separate from the frontend package if you want e2e tests to remain independently runnable.
