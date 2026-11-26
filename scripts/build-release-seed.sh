#!/usr/bin/env bash
# Creates a frozen release seed for the schema at the current checkout's HEAD.
# Write a new file for each supported installation's starting schema. Preserve
# earlier seeds so their tests exercise data written by the corresponding app.
#
# Output is not byte-reproducible: the book carries real timestamps, request
# UUIDs, and a session token hash. Two runs produce equivalent books, not
# identical files — which is another reason to freeze a fixture rather than
# regenerate it.
#
# Usage: ./scripts/build-release-seed.sh <output.sql>
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <output.sql>" >&2
  exit 2
fi
OUT="$(python3 -c 'import pathlib, sys; print(pathlib.Path(sys.argv[1]).resolve())' "$1")"
if [ -e "$OUT" ]; then
  echo "release seed output already exists: $OUT" >&2
  exit 1
fi
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

(SEED_OUT="$WORK/seed.sqlite" "$ROOT/scripts/go-backend.sh" test ./internal/api \
  -run '^TestBuildReleaseSeedFixture$' -count=1)

python3 - "$WORK/seed.sqlite" "$WORK/candidate.sql" <<'PY'
import pathlib, re, sqlite3, subprocess, sys

source, target = sys.argv[1], sys.argv[2]
dump = subprocess.run(["sqlite3", source, ".dump"], capture_output=True, text=True, check=True).stdout

# A dumped text value may contain newlines, so accumulate lines into whole
# statements rather than treating one line as one INSERT.
statements, current = [], None
for line in dump.splitlines():
    if current is None:
        if not line.startswith("INSERT INTO "):
            continue
        current = line
    else:
        current += "\n" + line
    if current.rstrip().endswith(");"):
        statements.append(current)
        current = None
if current is not None:
    raise SystemExit("unterminated INSERT in dump")

# Rows the migration inserts itself would collide; goose's bookkeeping belongs
# to the migration run; the FTS5 shadow tables are rebuilt by triggers.
SKIP = {"goose_db_version", "account_kinds", "market_data_sources"}
inserts, books = [], None
for statement in statements:
    table = statement.split()[2].strip('"')
    if table in SKIP or table.startswith("transaction_search") or table.startswith("sqlite_"):
        continue
    if table == "books":
        books = statement
    else:
        inserts.append(statement)
if books is None:
    raise SystemExit("dump has no books row")

# Restore the saved queue immediately after its book. Replaying account and
# posting triggers can then see their existing FX work and deduplicate it.
work_items = [s for s in inserts if s.split()[2].strip('"') == "background_work_items"]
inserts = work_items + [s for s in inserts if s.split()[2].strip('"') != "background_work_items"]

# FK deferral does not defer admission triggers. A lot's opening-operation
# guard needs its audited operation header already present; clearing allocations
# also need their journal links. Physical table order puts lots first. Move headers before lots, retaining row/id order.
header_tables = ("investment_operations", "investment_operation_dates", "investment_operation_journal_links")
operations = [s for table in header_tables for s in inserts if s.split()[2].strip('"') == table]
inserts = [s for s in inserts if s.split()[2].strip('"') not in header_tables]
lot_index = next((i for i, s in enumerate(inserts) if s.split()[2].strip('"') == "investment_lots"), len(inserts))
inserts[lot_index:lot_index] = operations

# books.default_currency_commodity_id is checked by a trigger on write, and
# triggers do not honour defer_foreign_keys — so the book goes in without it
# and is pointed at its currency once that row exists.
fields = re.match(r"INSERT INTO books VALUES\((.*)\);$", books, re.S).group(1).split(",")
currency, fields[4] = fields[4], "NULL"

connection = sqlite3.connect(source)
generation = connection.execute("PRAGMA user_version").fetchone()[0]
head = connection.execute("SELECT max(version_id) FROM goose_db_version WHERE is_applied = 1").fetchone()[0]
header = f"""-- Frozen synthetic book written through the HTTP API at migration {head:04d}
-- (baseline generation {generation}). Load after applying migrations through {head:04d}.
--
-- Preserve this file for upgrades from this schema. Create later fixtures in
-- new files with scripts/build-release-seed.sh.
--
-- Excluded: rows the migration inserts itself (account_kinds,
-- market_data_sources), goose's own bookkeeping, and the FTS5
-- shadow tables, which the triggers rebuild from the base rows below.
--
-- The loader defers foreign keys to COMMIT. Saved FX work and the book's
-- default currency are restored before account and posting triggers run.
"""
# Explicit column lists keep a frozen fixture's meaning independent of later
# additive columns, while still loading every original field verbatim.
def explicit_columns(statement):
    table = statement.split()[2].strip('"')
    columns = [row[0] for row in connection.execute("SELECT name FROM pragma_table_info(?)", (table,))]
    statement = statement.replace(" VALUES(", " (" + ", ".join(columns) + ") VALUES(", 1)
    if table == "setup_steps":
        # The migration pre-creates these rows. Restore their saved progress.
        statement = statement.removesuffix(";") + (
            " ON CONFLICT (step_key) DO UPDATE SET completed_at = excluded.completed_at,"
            " completed_audit_event_id = excluded.completed_audit_event_id;"
        )
    return statement
# Restore the default as soon as its commodity exists. Waiting until the end
# would let account/posting triggers enqueue foreign-currency work for the
# book's own currency while the pointer is temporarily NULL.
commodity_end = max(i for i, statement in enumerate(inserts)
                    if statement.split()[2].strip('"') == "commodities") + 1
statements = [explicit_columns(statement) for statement in
              ["INSERT INTO books VALUES(" + ",".join(fields) + ");"] + inserts]
statements.insert(commodity_end + 1, "UPDATE books SET default_currency_commodity_id = %s WHERE id = 1;" % currency)
body = "\n".join(statements)
connection.close()
pathlib.Path(target).write_text(header + body + "\n")
print("prepared %d statements" % len(statements))
PY

# Exercise the real consumer, including its production CHECKs, FKs, triggers,
# exact ledger/lot/checkpoint oracles and repeated migration admission. Publish
# a proposed fixture only after it loads; do not replace an installed fixture.
(SEED_INPUT="$WORK/candidate.sql" SEED_SOURCE="$WORK/seed.sqlite" "$ROOT/scripts/go-backend.sh" test ./internal/db \
  -run '^TestValidateReleaseSeedFixture$' -count=1)
(set -o noclobber; cat "$WORK/candidate.sql" > "$OUT")
echo "wrote validated seed: $OUT"
