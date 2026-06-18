# ADR 0009: Lossless Quantity Precision

## Status

Accepted

## Date

2026-06-18

## Context

The original signed-`int64` coefficient and 12-place scale ceiling cannot
represent common crypto quantities safely. At scale 18, an `int64` can hold
only about 9.22 whole units, and JSON numbers lose integer precision beyond
JavaScript's safe range.

## Decision

- Exact quantity coefficients are canonical signed base-10 strings in SQLite
  and JSON and use arbitrary-precision integers for backend arithmetic.
- Coefficients contain at most 38 decimal digits.
- The technical quantity-scale ceiling is 24.
- `crypto` commodities may use scales through 24. Currency, security, reward,
  and general commodity kinds remain capped at 12.
- Each commodity still declares its own `standard_scale` and
  `max_quantity_scale`; the kind ceiling is an upper bound, not a display
  default.
- Price and monetary cost coefficients remain separately constrained by their
  own models. Operations combining quantities and prices must round only at an
  explicit result boundary.

## Consequences

- BTC can normally use scale 8 and ETH scale 18 without forcing every crypto
  value to display 24 places.
- Browsers and API clients must treat quantity coefficients as strings and use
  exact decimal helpers rather than JavaScript `number` arithmetic.
- SQLite aggregation over text coefficients is forbidden; quantities are
  aggregated with arbitrary-precision integers in application read models.
- Existing integer JSON quantity inputs remain accepted by the backend for
  compatibility, but responses and the OpenAPI contract use strings.
