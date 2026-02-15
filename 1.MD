## Recommended Architecture

- Use one immutable price_observations fact table for stocks, FX, crypto (direct + derived), with provenance: source, provider symbol, fetched_at, valid_date/time, precision, ingest_run_id, and supersedes_observation_id for corrections.
- Keep price_sources and mappings, but add pricing_policy (source priority, staleness limits, triangulation rules, rounding mode, official-vs-market preference).
- Add valuation_provenance (or snapshot table) storing exact observation IDs/path used for each valuation/report line so any report can be exactly replayed.

## Update + Recompute Strategy

- Ingestion: append-only writes, never destructive INSERT OR REPLACE; corrections create new rows that supersede old.
- Canonical price resolution: deterministic engine (direct → inverse → triangulated path) with stable tie-breakers and path-cost penalties.
- Reports run in two modes: frozen (replay exact historical output) and latest_corrected (recompute with newest corrected observations).
Base Currency + Triangulation

## Base Currency + Triangulation

- Do not rewrite historical prices when base currency changes; keep native quote pairs and convert at query/report time.
- Store book_base_currency_history (effective date ranges) and include selected presentation currency + policy version in report metadata.
- Triangulation differences are normal; persist path + residual/error metric, and apply rounding only at final presentation step.

## Practical next steps (in your codebase)

- Add FX book_state bump triggers first (quick correctness win for cache invalidation).
- Replace FX/commodity upsert paths with append-only ingest + supersede semantics.
- Extend report_runs with pricing_policy_version + optional provenance snapshot ID.
- Then implement valuation service as a dedicated module and route report valuation through it.