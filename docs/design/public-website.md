# Public project website

Owner-approved direction, 2026-10-28; related adoption work is
[#335](https://github.com/sergeyfarin/rekenraam/issues/335). This is the
website slice, not acceptance of #335's demo and migration walkthrough.

## Presentation and copy

Use the approved Paper & pine direction: warm neutral surfaces, forest-green
accents, system fonts, restrained spacing and actual application screenshots.
Support light and dark appearances and mobile browsing. Do not add slogans,
testimonials, activity counters or claims of financial correctness or security
that the project cannot establish.

Describe the concrete differences: self-hosted data storage, no required
external account for core bookkeeping, double-entry accounting with exact
decimals, and detailed investment records. Explain optional provider requests,
operator responsibilities and the single-user boundary. Keep the pre-release
notice beside the introduction and retain development/upgrade limits, the
AI-use disclosure and the translation-review status. Source capability claims
from `docs/implemented.md`; the README can lag behind recent delivery.

The owner's copy revision groups features into four peers: self-hosting and
data control (including import/export and backup/restore), the double-entry
multi-currency ledger, investments and crypto, and budgets/reports. Each has
one description. Omit a summary paragraph that repeats these groups, generic
section introductions, and a second inventory of investment methods beneath
screenshots. Keep deployment instructions, project limits and translation
review notes concise and separate from the feature descriptions.

The main screenshot area is a manual carousel containing the spending report,
transaction register and investment positions, with numbered selection,
previous/next controls and full-image links. Never rotate automatically.
Announce selections without moving keyboard focus. Without JavaScript, display
all three figures with their captions and links.

The retirement planner at `retirement.rekenraam.com` is a separate tool. Link
to it without implying that it shares a login, database or hosting lifecycle.

## Static distribution

`website/` is a separate, dependency-free static website in the app repository.
A Node build renders complete HTML from `website/locales/*.json`, with a shared
template and semantic CSS tokens. This small content site does not need a
second SvelteKit application or a running Go/API server. It does not change the
app's Go/SvelteKit/SQLite production shape or its Docker image.

The app's `frontend/project.inlang/settings.json` owns the published locale set.
English is at `/`, Dutch at `/nl/` and Russian at `/ru/`. Spanish, French
and German catalogs stay in source. All website catalogs must match the English
keys and placeholders and contain nonempty strings. All page copy, metadata,
captions, alternative text, notices and controls come from these catalogs.
Language selection uses ordinary links and works without JavaScript. Native
review for the website and app catalogs remains with #102.

Load all website assets from the site's own origin. Do not add analytics,
tracking scripts, remote fonts, forms for financial data or app API calls.
Local runtime scripts switch appearance and screenshots; only the appearance
preference uses local browser storage. Hosting-provider access logs remain
outside this claim.
Screenshots are English and explicitly labeled as fictional sample data.

The site has no interactive app demo yet. A future demo must distinguish
supported sample-book interactions from the real app and preserve the
provider-independent/privacy requirements in #335. Preparing this website does
not satisfy that issue or authorize a public launch.

## Hosting boundary

The initial target is GitHub Pages at `rekenraam.com`. Build and publication
are separate: normal CI checks/builds the site; the Website workflow defaults
to a review artifact without deployment. Its explicit `publish` input is for
an owner-authorized publication after Pages settings and DNS are configured.
Preserve the retirement subdomain's DNS and hosting. See
`docs/developer-workflow.md` → Public website for commands and setup.
