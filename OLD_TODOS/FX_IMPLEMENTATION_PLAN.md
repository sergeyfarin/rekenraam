FX Rates: Implementation Plan (Historical + Daily Refresh)

Goals
- Track historical FX rates to value assets/liabilities in the base currency across time.
- Match or exceed common finance apps: daily rates, historical backfill, configurable sources, and auditability.
- Support per-currency (or per-pair) data sources with effective date ranges.
- Allow changing sources over time while preserving history.
- Daily refresh: download only missing days since last successful update.
- Handle non-USD base currencies with robust cross-rate derivation.
- Keep most currencies inactive by default (active: USD, EUR, GBP). Inactive = no automatic refresh.

Key Capabilities (Target Parity+)
1) Historical daily rates per currency pair
	- Store one rate per day per pair with source attribution.
	- Fetch missing ranges on demand and via scheduled refresh.
2) Configurable sources
	- Default global source and per-currency overrides.
	- Source assignments are time-bound to allow historical source changes.
3) Cross-rate support for non-USD base
	- Pivot/triangulate through USD or EUR when direct rates are absent.
	- Persist derived rates with explicit provenance (source + derivation method).
4) Refresh automation
	- Daily job and manual “Refresh now” action.
	- Backfill: fetch from last_success_date+1 to today (or last business day).
5) Auditability
	- Each rate stores source_id and source name.
	- Refresh state records last attempt, success date, and error.

Design Decisions
- Storage model supports source changes without rewriting historical data.
- Rates stored as “from currency → base currency” for valuation; cross rates derived when needed.
- Store rate_date as YYYY-MM-DD to align with provider data and reporting.
- Weekend handling policy per book: skip, fill previous, or download.

Data Model (Additions)
1) fx_rate_settings
	- base_currency_id
	- default_source_id
	- refresh_enabled
	- refresh_hour_utc / refresh_minute_utc
	- max_backfill_days
	- weekend_policy (skip | fill_previous | download)

2) fx_rate_source_assignments
	- from_currency_id, to_currency_id, source_id
	- effective_from, effective_to
	- Enables source changes over time

3) fx_rate_refresh_state
	- from_currency_id, to_currency_id, source_id
	- last_success_date, last_attempt_at, last_error
	- Used to fetch only missing days

4) fx_rates_daily
	- add source_id (nullable) for stronger provenance
	- add is_derived + derived_via_currency_id to distinguish downloaded vs derived pairs

Source Strategy
- Default source (daily market rates): ECB (EUR base) or Federal Reserve (USD base).
- Optional future: Frankfurter (ECB API), Bank of England, Bank of Canada, RBA, SNB.
- Official/tax rates already supported (monthly/yearly) via fx_rates_official.

Cross-Rate Handling (Non-USD Base)
- If source provides only base=USD or base=EUR rates:
  1) Prefer direct pair if available.
  2) Else triangulate: rate(A→B) = rate(A→USD) × rate(USD→B), or via EUR.
  3) Persist derived rates with a distinct source label like “derived:USD/ECB”.
- Ensure derived rates are only used when direct is missing for that date.

Currency Activation Policy
- Seed USD, EUR, GBP as active; all others inactive by default.
- Only active currencies are included in daily refresh.
- UI should make activation explicit and show refresh impact.

Refresh Workflow
1) Determine base currency (fx_rate_settings.base_currency_id).
2) Determine active currencies (commodities where kind='currency' and is_active=1).
3) For each active currency pair (from_currency → base):
	- Resolve source for date range:
	  - Find assignment effective for date; fall back to default_source_id.
	- Read fx_rate_refresh_state to determine last_success_date.
	- Fetch missing dates (bounded by max_backfill_days).
4) Store rates (fx_rates_daily) with source_id and source name.
5) Update refresh_state with last_success_date and errors.

UI/UX Plan (Settings)
- FX Settings panel:
  - Base currency selection
  - Default source selection
  - Refresh schedule + weekend policy
  - “Refresh now” button with progress
- Currency list:
  - Active toggle notes “inactive = no FX refresh”
- Source assignments:
  - Per-currency overrides with effective date range
  - History list with edit/delete

Implementation Steps
Phase 1: Storage + Commands (STARTED)
- Add tables for fx_rate_settings, fx_rate_source_assignments, fx_rate_refresh_state.
- Add source_id to fx_rates_daily.
- Add is_derived + derived_via_currency_id to preserve downloaded pairs alongside derived/base pairs.
- Add Tauri commands to read/update FX settings, source assignments, and refresh state.
- Update currency seed defaults to keep most currencies inactive.

Phase 2: Source Fetchers
- Implement a provider interface in Rust:
  - fetch_daily_rates(base, symbols, start_date, end_date)
  - returns rate map with source metadata
- Add ECB + Federal Reserve adapters.
- Add Yahoo and 2-3 more daily FX sources (candidates: Frankfurter/ECB API, Bank of England, Bank of Canada, RBA, SNB).
- Add derivation logic for missing cross rates.

Phase 3: Refresh Engine
- Background scheduler (daily) + manual refresh command.
- Incremental backfill using fx_rate_refresh_state.
- Resilient error handling and partial successes.

Phase 4: UI Integration
- FX Settings panel and source assignment editor.
- Refresh status view and last-success indicators.

Phase 5: Reports/Valuation
- Ensure valuation uses fx_rates_daily by date.
- Show FX impact and unrealized FX gains/losses in reports.

Status
- Phase 1 implemented: schema + commands + default currency activation behavior.
- Phase 2 implemented: provider interface + adapters (ECB/Frankfurter, Fed/FRED, Yahoo, ExchangeRate.host, Bank of Canada) + cross-rate derivation.
- Phase 3 implemented: refresh engine (manual command + scheduler + incremental backfill + error tracking).
- Phase 4 implemented: FX settings UI, source assignment editor, refresh status view.

Next
- Add derivation + fallback policy for base currency not USD.
- Add derivation + fallback policy for base currency not USD.
