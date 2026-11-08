# Rekenraam project website

The Paper & pine project site, with complete static pages in the app's six
languages. Product/design decisions are in
[the website contract](../docs/design/public-website.md); current capabilities
are in [the feature ledger](../docs/implemented.md).

Node 24 is sufficient; this project has no npm dependencies. Use the root pnpm
commands:

```sh
pnpm website:dev           # build, serve on 127.0.0.1:1889, watch copy/CSS/assets
pnpm website:check         # locale parity, rendered pages, links and base paths
pnpm website:build         # website/build/, independent of the app build
pnpm website:preview       # serve the existing build on 127.0.0.1:1889
pnpm website:check-browser # six locales, mobile/desktop, axe, themes, no-JS links
```

Restart the dev command after changing `build.mjs`; catalog and asset edits
rebuild automatically. Change the loopback preview port with `WEBSITE_PORT`.
Browser checks use the fixed test port 1889 and require the existing E2E
dependencies and Playwright Chromium. Stop a running preview before the checks.

Edit `locales/en.json` first, then all other catalogs, following the
[localization glossary](../docs/localization-glossary.md). Catalog parity is a
build requirement; it is not native-language approval. The website has its own
translation boundary and never downloads translation code or content.

All routes, language links and content work without JavaScript. CSS adapts to
system appearance; `assets/theme.js` adds a local theme preference when scripts
and storage are available. `assets/gallery.js` adds manual screenshot navigation
without automatic rotation; all three screenshots remain visible without
JavaScript. The website has no analytics or external assets.

Default output targets `https://rekenraam.com`. To check a project-path build:

```sh
SITE_ORIGIN=https://sergeyfarin.github.io SITE_BASE_PATH=/rekenraam pnpm website:build
SITE_BASE_PATH=/rekenraam pnpm website:preview
```

For custom-domain builds leave both variables unset. Setup and the manual
review/publish workflow are documented in
[Public website](../docs/developer-workflow.md#public-website).

## Screenshot sources

- All three screenshots are captured from an app instance with a new
  disposable database, replaying `fixtures/reference-book.json` through public
  APIs without a run suffix. The capture uses no real owner database,
  credentials or providers. Source revision and viewports are recorded in
  `assets/capture.json`, which is not published.
- The spending report shows the book's euro spending (one commodity, so its
  chart is drawn). It is written to `docs/images/reports-spending.png` at 2x
  (2880 × 2224, the size `build.mjs` states) and copied during build. It is the
  same screenshot the repository README shows.
- Capture after committing the code change, then commit the images:
  `capture.json` records `source_revision` and `source_clean`, which is true
  only when nothing but these images differed from that revision.
- `assets/transactions.png` and `assets/investments.png` are 1440 × 1050.

Refresh deliberately after building the current app binary:

```sh
pnpm build
pnpm --dir e2e exec playwright test --config=website.config.ts
```

The capture command starts a disposable app at port 16980, shuts it down and
deletes its temporary database. It refuses `E2E_BASE_URL`. Screenshots show the
English app; every site language explains that and supplies translated alt text.
It is an asset generator, separate from normal app/browser tests.
