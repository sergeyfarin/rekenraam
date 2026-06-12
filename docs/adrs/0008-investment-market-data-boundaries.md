# ADR 0008: Investment And Market Data Boundaries

## Status

Accepted

## Date

2026-06-12

## Context

Rekenraam needs investment support for shares and ETFs without weakening the
core ledger model. Investments introduce security identity, quote prices,
dividends, lots, cost basis, and market-data provider ingestion. Provider data
can be incomplete or wrong, while posted ledger data must remain explainable and
auditable.

The archived Python-era design included richer market-data ingestion and
corporate-action concepts than a simple holdings table. Those ideas are adopted
only after translating them into the active Go, SQLite, and browser API shape.

## Decision

Investments extend the existing commodity, account, and double-entry ledger
model.

- Securities use `commodities.kind = 'security'` and get investment instrument
  identity/version rows for symbols, exchange/MIC data, identifiers, issuer,
  quote/trading commodity, scale, status, and provider metadata.
- Security holding accounts remain asset accounts with
  `account_kind = 'security_holding'` and a security commodity as their default
  commodity. Brokerage cash remains ordinary currency-denominated asset
  accounts.
- Buy, sell, dividend, and reinvested-dividend services post ordinary ledger
  transactions. Security trades use the `commodity_trading` system account so
  each commodity remains balanced.
- Prices, FX rates, provider prices, manual prices, and trade-implied prices
  are exact integer-plus-scale observations. Observations are superseded or
  voided rather than overwritten.
- Lots are first-class from the first investment slice. FIFO is the default
  implemented disposal method, while the schema preserves policy values for
  `fifo`, `lifo`, `average_cost`, and `specific_lot`.
- Provider adapters are pluggable. Provider secrets live in operator
  configuration, not SQLite business rows.
- Provider events become reviewable suggestions by default. Trusted sources may
  auto-post only through explicit automation rules with source/instrument/event
  scope, confidence threshold, required account mappings, effective dates, and
  audit attribution.
- Complex corporate actions may be stored and suggested before deterministic
  posting is implemented.

## Consequences

- Investment workflows reuse ledger audit, balancing, correction, and export
  foundations instead of inventing a separate portfolio ledger.
- Market data can be refreshed or corrected without rewriting posted trades.
- Future realized-gain reports can derive from lot events without
  reinterpreting historical buys and sells.
- Provider ingestion requires careful idempotency and suggestion review tests.
- OpenAPI contracts, frontend workflows, and provider adapters can evolve in
  slices, but must preserve the trust boundary that external data is not posted
  silently unless an explicit automation rule allows it.
