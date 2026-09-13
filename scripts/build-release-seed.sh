#!/usr/bin/env bash
# Regenerates a frozen release seed fixture for the historical-upgrade test.
#
# The fixture stands in for "a database written by a released version of the
# app". internal/db's TestMigrateUpgradesV01DatabaseToFreshHeadSchema replays it
# into a database migrated only to that release's baseline, upgrades it to HEAD,
# and asserts every durable figure survived.
#
# EXISTING FIXTURES ARE FROZEN. Regenerating v01_seed.sql against a later schema
# defeats the test it feeds: the point is to migrate data written by the OLD
# app, which current code may no longer be able to produce. Use this to create
# the seed for a NEW release, into a new file.
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
OUT="$1"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

(cd "$ROOT/backend" && SEED_OUT="$WORK/seed.sqlite" go test ./internal/api \
  -run '^TestBuildReleaseSeedFixture$' -count=1)

python3 - "$WORK/seed.sqlite" "$OUT" <<'PY'
import pathlib, re, subprocess, sys

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
SKIP = {"goose_db_version", "account_kinds", "market_data_sources", "setup_steps"}
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

# books.default_currency_commodity_id is checked by a trigger on write, and
# triggers do not honour defer_foreign_keys — so the book goes in without it
# and is pointed at its currency once that row exists.
fields = re.match(r"INSERT INTO books VALUES\((.*)\);$", books, re.S).group(1).split(",")
currency, fields[4] = fields[4], "NULL"

header = """-- Frozen v0.1 seed: a realistic book built through the real HTTP API against
-- migration 0001, then dumped. TestMigrateUpgradesV01DatabaseToFreshHeadSchema
-- replays it into a 0001-only database and upgrades that to HEAD, so this file
-- stands in for "a database written by a released version".
--
-- FROZEN. Regenerating this against a later schema would defeat the test: the
-- point is to migrate data written by the OLD app, which current code may no
-- longer be able to produce. Add a second seed for a later release instead.
-- Rebuild with scripts/build-release-seed.sh.
--
-- Excluded: rows the migration inserts itself (account_kinds,
-- market_data_sources, setup_steps), goose's own bookkeeping, and the FTS5
-- shadow tables, which the triggers rebuild from the base rows below.
--
-- Two ordering accommodations, because a dump is in table order and a book is
-- not a tree: the loader defers foreign keys to COMMIT, and the book's default
-- currency is set at the end, once the commodity it points at exists.
"""
body = "\n".join(["INSERT INTO books VALUES(" + ",".join(fields) + ");"] + inserts)
footer = "\nUPDATE books SET default_currency_commodity_id = %s WHERE id = 1;\n" % currency
pathlib.Path(target).write_text(header + body + footer)
print("wrote %s (%d statements)" % (target, len(inserts) + 1))
PY
