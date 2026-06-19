# Rekenraam — Code Quality Backlog

Status legend: `[ ]` open · `[x]` done · `[~]` won't fix / by design

Issues are verified against the actual code. Each entry includes the exact file and
line so it can be opened and fixed without re-reading the analysis.

---

## Precision & Arithmetic

### B-01 `scaledDivision` truncates instead of rounding  `[x]`
**File:** `backend/internal/app/investments.go:1576–1592`  
**Call site:** `backend/internal/app/pricing.go:282`

`n.Div()` (floor division) followed by `n.Quo()` (truncate-toward-zero division) are
applied in sequence. For trade-implied prices that produce repeating decimals the
stored price is systematically off by up to 1 ULP at `resultScale`.

Example: cash=100 USD (scale 2), quantity=3 BTC (scale 8), resultScale=8
→ stored 3333333333 (33.33333333 USD/BTC), correct is 33.33333333… — error ≈ 3e-9.
Small per trade, but it compounds across many trades and breaks exact audit trails.

**Fix:** after scaling `n` up, use `big.Int.DivMod` or `QuoRem` and apply half-up
rounding when the remainder × 2 ≥ divisor before calling `IsInt64()`.

---

### B-02 `pow10` has no upper-bound guard  `[x]`
**File:** `backend/internal/app/transactions.go:917–924`  
(same function also called from `ledger.go` and `reconciliation.go` indirectly via
`scaledAmount.align`)

```go
func pow10(scale int) *big.Int {
    result := big.NewInt(1)
    ten := big.NewInt(10)
    for i := 0; i < scale; i++ {
        result.Mul(result, ten)
    }
    return result
}
```

`scale` is never validated before this call. Input postings are validated to
`MaxCryptoScale = 24` at the API boundary, so normal paths are safe. But
`scaledAmount.align()` computes `scale - a.scale` which could be larger than 24 if
two postings for the same commodity arrive at very different scales (currently
impossible due to per-commodity max validation, but fragile).

**Risk:** `scaledDivision` also calls `pow10(exponent)` where
`exponent = resultScale + denominatorScale - numeratorScale`. With
`resultScale=12, denominatorScale=24, numeratorScale=0` → `exponent=36`. Still
handled correctly by `big.Int`, but if `exponent` were ever negative a no-op loop
silently returns 1 which is wrong.

**Fix:** Add `if scale < 0 { panic("pow10: negative scale") }` and a reasonable
upper-bound assertion (e.g. `scale > 60` is a bug, not data). Also add a negative
guard to `align()`.

---

### B-03 Crypto-scale precision accumulation: 38-digit limit hit at query time, not write time  `[ ]`
**File:** `backend/internal/app/ledger.go:482–484` (`coefficient()`)

`scaledAmount` accumulates via `*big.Int` (unbounded). The 38-digit limit from
`exact.Parse` is only enforced when `coefficient()` is called at the end of a ledger
query. This means:

- A large crypto holding posted legitimately at scale 24 uses up to 38 digits for the
  coefficient. Accumulated across N postings the raw `big.Int` may exceed 38 digits.
- `coefficient()` then returns an error and the ledger/register request fails with
  an HTTP 500.
- The data is valid; the aggregation math itself is the problem.

**Impact:** users with large crypto balances (many small postings adding up to > 10^14
tokens) will see the account balance page return 500 with no useful message.

**Fix:** Either (a) enforce a maximum total-digits limit on individual postings so the
sum can never overflow the budget, or (b) extend `MaxCoefficientDigits` for ledger
aggregation only (it's already a constant), or (c) improve the error message so the
user sees a comprehensible explanation. Option (c) is the minimum safe fix.

---

### B-04 `scaledAmount.add()` mutates intermediate `big.Int` from `BigInt()`  `[ ]`
**File:** `backend/internal/app/ledger.go:453–460`

```go
func (a *scaledAmount) add(value exact.Coefficient, scale int) {
    a.align(scale)
    addend := value.BigInt()          // returns new(big.Int) — a fresh copy ✓
    if scale < a.scale {
        addend.Mul(addend, pow10(a.scale-scale))   // mutates the copy ✓
    }
    a.value.Add(a.value, addend)
}
```

`Coefficient.BigInt()` creates a fresh `*big.Int`, so `addend.Mul(...)` does **not**
corrupt the source `Coefficient`. This is safe as written.

**Contrast with `addScaled`** (line 462–471): that method correctly does
`addend := new(big.Int).Set(other.value)` to copy the mutable `*big.Int` from the
other `scaledAmount` before mutating. The asymmetry is correct because `Coefficient`
is immutable by value but `scaledAmount.value` is a shared pointer.

Status: `[~]` — no bug, documented here because the audit flagged it.

---

## Error Handling & Observability

### B-05 `writeJSON` silently drops encode errors  `[x]`
**File:** `backend/internal/api/middleware.go:35`

```go
_ = json.NewEncoder(w).Encode(payload)
```

If `json.Encoder.Encode` fails (e.g., a handler mistakenly puts a non-serialisable
value in a response struct, or the connection is dropped after headers are sent), the
error is discarded. The client receives a truncated or empty body; the logs show no
error; the response status is already written so `withRecovery` cannot send a clean
error response.

**Fix:** log the error at warn level with request ID. Because the header is already
sent, there is nothing else to do at the transport layer, but at least the failure
becomes visible in logs.

---

### B-06 `tx.Rollback()` errors silently ignored in all DB transaction helpers  `[x]`
**File:** `backend/internal/db/transactions.go:356, 429, 504, 597`

```go
defer func() {
    if !committed {
        _ = tx.Rollback()
    }
}()
```

A rollback error (e.g., connection dropped mid-transaction on a WAL checkpoint) is
swallowed. SQLite in WAL mode self-recovers on restart, so this is unlikely to cause
persistent corruption, but the silence makes debugging harder and hides genuine
connectivity problems.

**Fix:** log rollback errors at error level, including the original transaction
context (function name + request ID if available).

---

### B-07 JSON encode error also silently dropped in reconciliation preview handler  `[ ]`
**File:** Search `writeJSON` call sites — all share the same `middleware.go:35` path.  
Covered by B-05 fix.

---

## Security

### B-08 FTS5 query — `ftsPhrase` must sanitise FTS5 operator syntax  `[x]`
**File:** `backend/internal/db/transactions.go:259–264`

```go
where = append(where, `tv.id IN (
    SELECT CAST(transaction_version_id AS INTEGER)
    FROM transaction_search
    WHERE transaction_search MATCH ?
)`)
args = append(args, ftsPhrase(params.Query))
```

The `?` placeholder prevents SQL injection. But FTS5 has its own operator language
(`AND`, `OR`, `NOT`, `^`, `"phrase"`, column filters). If `ftsPhrase()` passes the
raw user string, a user can craft queries that trigger FTS5 parse errors (HTTP 500)
or deliberately slow FTS scans.

**Verify:** read `ftsPhrase()` implementation and confirm it wraps the input in
double-quotes and escapes interior double-quotes, which is the correct FTS5 phrase
quoting strategy.

---

### B-09 Login timing leak: "user not found" fast-paths before bcrypt  `[x]`
**File:** `backend/internal/app/auth.go` (login path)

When the owner account does not exist the function returns early without calling
`bcrypt.CompareHashAndPassword`. A remote attacker can distinguish "no such user"
from "wrong password" by measuring response time.

**Severity:** Low for a single-owner self-hosted app (the attacker would already know
there is an account), but worth fixing for defence-in-depth.

**Fix:** call a dummy `bcrypt.CompareHashAndPassword` against a static hash before
returning when the user is not found.

---

### B-10 `recover-owner` reads password from stdin with echo enabled  `[x]`
**File:** `backend/cmd/rekenraam/command.go` (recover-owner command)

`io.ReadAll(os.Stdin)` leaves terminal echo on, so the password is visible on screen
and in terminal scrollback.

**Fix:** use `golang.org/x/term.ReadPassword(int(os.Stdin.Fd()))`.

---

## API Contract / Schema Drift

### B-11 `InvalidatedCheckpointIDs` uses `omitempty` but schema contract is unclear  `[ ]`
**File:** `backend/internal/api/transactions.go:35`

`InvalidatedCheckpointIDs []int64 \`json:"invalidated_checkpoint_ids,omitempty"\``

When the slice is nil/empty this field is absent from the JSON response.
The OpenAPI schema (`transactions.yaml:192–196`) marks it as optional and not in
`required`, so `omitempty` is technically correct. However, the TypeScript client
generated by `openapi-typescript` will type it as `number[] | undefined`, meaning
frontend code must guard every access with `?? []`.

**Decision needed:** either always emit `[]` (remove `omitempty`) for consistency with
`tag_ids` / `transaction_tag_ids` which always emit arrays, or explicitly document the
absent-vs-empty distinction in the schema.

---

### B-12 Trade-implied price pagination never consumed by frontend  `[ ]`
**File:** `frontend/src/lib/api/transactions.ts` (and reconciliation equivalent)

Backend list endpoints return `next_cursor`. Frontend fetches once with
`staleTime: 5_000` and ignores `next_cursor`. Users with many transactions only see
the first page with no indication that more results exist.

**Fix:** implement `useInfiniteQuery` or expose a "load more" control; at minimum
show a count of hidden results.

---

## Business Logic

### B-13 `FinishReconciliation` does not assert difference is zero  `[ ]`
**File:** `backend/internal/app/reconciliation.go:296–325`

`FinishReconciliationSession` is called without first asserting
`session.Difference.QuantityValue == 0`. A race condition (two concurrent requests)
or direct DB edit could finalise an unbalanced reconciliation.

**Verify:** check whether the DB layer has a CHECK constraint or trigger that enforces
this. If not, add the assertion in the service layer before calling the repository.

---

### B-14 Draft transactions bypass double-entry structural validation  `[ ]`
**File:** `backend/internal/app/transactions.go:682–694`

`validateBalanced` and the ≥2-postings check are guarded by `if status == "posted"`.
A draft can be saved with 1 posting and an unbalanced entry. While the
draft→posted transition will catch this, any code path that reads or displays drafts
will encounter structurally invalid journal entries.

**Decision needed:** reject structurally invalid drafts at save time (recommended), or
accept the current behaviour and ensure all draft-reading code handles degenerate
entries.

---

### B-15 Dividend withholding can silently mix commodities  `[ ]`
**File:** `backend/internal/app/investments.go:887–898`

The dividend withholding posting is created using its own scale and value without
asserting it shares the same commodity as the dividend amount posting. A configured
default withholding in a different commodity would produce a journal entry that
appears balanced per-commodity but has wrong economic meaning.

**Fix:** validate that `WithholdingCommodityID == DividendCommodityID` (or that
withholding is explicitly cross-currency and both postings are expected).

---

## Maintainability

### B-16 `unsafe-inline` in CSP is a known weakness  `[~]`
**File:** `backend/internal/api/middleware.go:72`

`script-src 'self' 'unsafe-inline'` is present because SvelteKit injects an inline
bootstrap script. A comment documents the planned fix (compute SHA-256 hash at build
time). Not actionable until the build pipeline is updated.

---

### B-17 No unit tests for `scaledAmount` arithmetic  `[x]`
**File:** `backend/internal/app/ledger.go:444–484`

`scaledAmount.add`, `.addScaled`, `.align`, and `coefficient` have no dedicated tests.
They are exercised only through full HTTP integration tests which makes precision
edge cases (e.g., crypto scale 24 alignment, large sums near 38-digit limit) hard to
catch in CI.

**Fix:** add a `ledger_math_test.go` in `backend/internal/app/` with table-driven
tests covering: scale alignment, sum overflow to >38 digits, negative values, and
same-commodity mixed-scale accumulation.

---

### B-18 No unit tests for `scaledDivision` / `pow10`  `[x]`
**File:** `backend/internal/app/investments.go:1576`, `transactions.go:917`

Same gap as B-17. `scaledDivision` is only called from one place
(`pricing.go:282`) and has no tests for truncation behaviour, overflow detection, or
division-by-zero guard.

---
