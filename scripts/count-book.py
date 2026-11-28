#!/usr/bin/env python3
"""Count the shape of a real book from a backup copy (#449, review D11).

The navigation decisions rest on estimated entry counts. This script measures
them on a copy of the owner's book so the switcher grouping and the pin limit
of #442 can be fixed against real numbers:

    python3 scripts/count-book.py /path/to/copy-of-backup.sqlite
    python3 scripts/count-book.py --json --window-end 2026-11-28 copy.sqlite

It prints counts only: no names, codes, symbols, notes or amounts, and every
label it prints is a fixed schema code (account class, kind, status). The
result is safe to paste into the review.

Run it on a copy of a backup, never on the live database. It follows symbolic
links to the real file and refuses one that has a `.lock`, `-wal` or `-shm`
file beside it (the server's lock file stays next to a live database even
while the server is stopped) or a second hard-linked name. It opens the copy
read-only and immutable, so SQLite writes nothing, not even a journal.

The output has two parts. `snapshot` counts the objects as they stand in the
backup: each object's version in force on the owner's today, and the open lots
it holds. `postings_in_window` counts postings in the 90 days ending on
`--window-end` (default: the owner's today); that option moves nothing else.
The owner's today is the date now in the time zone saved in the backup, or the
UTC date when none is saved, as the app's OwnerToday has it, not the date of
the machine running the script.

Definitions follow the app:
- an object's facts are its version in force on the owner's today;
- a category is an income or expense account whose kind equals its class and
  whose metadata says so (db.categoryAccountPredicate);
- holdings are security_holding and fund_holding accounts; a brokerage is one
  account-index row with its cash and holdings inside it, and a holding with no
  brokerage above it is its own row (app.LedgerOverview);
- a posting counts when its transaction's current version is posted, the
  transaction is not deleted, and its journal entry date falls in the window;
- an instrument is held when the backup has an open lot of it with quantity
  left (long or short);
- pins are offered from active account-index rows that are not holdings
  (review D7), a brokerage counting the postings of its cash and holdings.

Standard library only, so it runs wherever Python 3 does.
"""
import argparse
import datetime as dt
import json
import os
import sqlite3
import sys
import urllib.parse
import zoneinfo

SCRIPT_VERSION = 3
WINDOW_DAYS = 90
# The earliest window end whose whole window is a real date.
EARLIEST_WINDOW_END = dt.date.min + dt.timedelta(days=WINDOW_DAYS - 1)
HOLDING_KINDS = {"security_holding", "fund_holding"}
BALANCE_SHEET = {"asset", "liability"}
# Posting-count buckets for the pin suggestion: (label, lowest count).
BUCKETS = [("0", 0), ("1-4", 1), ("5-19", 5), ("20-49", 20), ("50-99", 50), ("100+", 100)]
REQUIRED_TABLES = {
    "goose_db_version",
    "accounts",
    "account_versions",
    "institutions",
    "institution_versions",
    "payees",
    "payee_versions",
    "tags",
    "investment_instruments",
    "investment_instrument_versions",
    "investment_lots",
    "investment_lot_state",
    "transactions",
    "journal_entries",
    "posting_versions",
    "user_preferences",
}


class Refused(Exception):
    """The file is not a safe, recognisable backup copy."""


def open_copy(given):
    # SQLite finds a database's sidecars by the name it opens, so check and
    # open the real file: a symbolic link elsewhere has no sidecars beside it.
    path = os.path.realpath(given)
    if not os.path.isfile(path):
        raise Refused(f"{given} is not a file")
    for suffix in (".lock", "-wal", "-shm"):
        if os.path.exists(path + suffix):
            raise Refused(
                f"{path}{suffix} exists, so this looks like a live database. "
                "Copy a backup file to another folder and count the copy."
            )
    # A hard link is the same file under a second name, which may be the live
    # one with its sidecars; a copy has one name.
    if os.stat(path).st_nlink > 1:
        raise Refused(f"{path} has another name (a hard link), so it may be a live database. Count a copy.")
    uri = "file:" + urllib.parse.quote(path) + "?mode=ro&immutable=1"
    try:
        conn = sqlite3.connect(uri, uri=True)
        conn.execute("PRAGMA query_only = ON")
        tables = {row[0] for row in conn.execute("SELECT name FROM sqlite_master WHERE type IN ('table', 'view')")}
    except sqlite3.DatabaseError as err:
        raise Refused(f"{path} is not a readable SQLite database: {err}") from err
    missing = REQUIRED_TABLES - tables
    if missing:
        conn.close()
        raise Refused(f"{path} is not a Rekenraam database (missing {', '.join(sorted(missing))})")
    return conn


def current_versions(conn, table, key, as_of, columns):
    """The version of each object in force on as_of; the first version when none is yet."""
    rows = conn.execute(
        f"SELECT {key}, effective_from, {', '.join(columns)} FROM {table} ORDER BY {key}, effective_from, version_seq"
    ).fetchall()
    result = {}
    for object_id, effective_from, *values in rows:
        # The app's order (effective_from, then version_seq), so the last row
        # in force wins; an object not in force yet keeps its first version.
        if object_id not in result or effective_from <= as_of:
            result[object_id] = dict(zip(columns, values))
    return result


def owner_today(conn):
    """The owner's calendar day now: the saved zone, or UTC without one (app.OwnerTodayAt)."""
    row = conn.execute(
        """
        SELECT preferences.time_zone
        FROM books LEFT JOIN user_preferences preferences ON preferences.user_id = books.owner_user_id
        ORDER BY books.id LIMIT 1
        """
    ).fetchone()
    zone = row[0] if row and row[0] else "UTC"
    try:
        location = zoneinfo.ZoneInfo(zone)
    except (zoneinfo.ZoneInfoNotFoundError, ValueError) as err:
        # The app refuses an unloadable zone too; the runner's date is not a
        # stand-in for the owner's. The zone is not printed: it says where the
        # owner lives.
        raise Refused(
            "cannot load the owner's time zone saved in the backup. If Python has no time zone "
            "database here, install one (for example `pip install tzdata`) and run again."
        ) from err
    return dt.datetime.now(location).date().isoformat()


def canonical_day(text):
    try:
        return dt.date.fromisoformat(text).isoformat() == text
    except ValueError:
        return False


def bump(counter, *keys):
    node = counter
    for key in keys[:-1]:
        node = node.setdefault(key, {})
    node[keys[-1]] = node.get(keys[-1], 0) + 1


def distribution(counts):
    result = {label: 0 for label, _ in BUCKETS}
    for count in counts:
        label = [label for label, low in BUCKETS if count >= low][-1]
        result[label] += 1
    return result


def is_category(version):
    if version["account_class"] not in ("income", "expense"):
        return False
    if version["account_kind"] != version["account_class"]:
        return False
    try:
        metadata = json.loads(version["metadata_json"] or "{}")
    except ValueError:
        return False
    category = metadata.get("category") if isinstance(metadata, dict) else None
    return isinstance(category, dict) and category.get("type") == version["account_class"]


def count_book(conn, snapshot_day, window_end):
    window_start = (dt.date.fromisoformat(window_end) - dt.timedelta(days=WINDOW_DAYS - 1)).isoformat()
    report = {
        "script_version": SCRIPT_VERSION,
        "schema_version": conn.execute("SELECT MAX(version_id) FROM goose_db_version").fetchone()[0],
        "books": conn.execute("SELECT COUNT(*) FROM books").fetchone()[0],
    }
    snapshot = {"versions_in_force_on": snapshot_day}
    as_of = snapshot_day

    system_roles = dict(conn.execute("SELECT id, system_role FROM accounts").fetchall())
    versions = current_versions(
        conn,
        "account_versions",
        "account_id",
        as_of,
        ["account_class", "account_kind", "status", "parent_account_id", "institution_id", "allows_postings", "metadata_json"],
    )
    accounts = {}
    for account_id, version in versions.items():
        role = system_roles.get(account_id)
        if role is not None:
            group = "system"
        elif is_category(version):
            group = "category"
        elif version["account_kind"] in HOLDING_KINDS:
            group = "holding"
        elif version["account_class"] in BALANCE_SHEET:
            group = "money"
        else:
            group = "other"
        accounts[account_id] = dict(version, group=group)

    # Ownership as the Ledger overview assigns it: the nearest brokerage
    # ancestor, otherwise the account itself, and a holding falls back to its
    # nearest non-holding ancestor.
    ledger = {i: a for i, a in accounts.items() if a["group"] in ("money", "holding")}

    def owner_for(account_id):
        fallback, seen, current = account_id, set(), account_id
        while current in ledger and current not in seen:
            seen.add(current)
            if ledger[current]["account_kind"] == "brokerage":
                return current
            if ledger[fallback]["group"] == "holding" and ledger[current]["group"] != "holding":
                fallback = current
            current = ledger[current]["parent_account_id"]
        return fallback

    owners = {i: owner_for(i) for i in ledger}
    index_rows = sorted(set(owners.values()))

    by_group = {}
    for a in accounts.values():
        bump(by_group, a["group"], a["status"])
    postable = {}
    for a in accounts.values():
        if a["group"] in ("money", "holding", "other") and a["allows_postings"]:
            bump(postable, a["account_class"], a["account_kind"], a["status"])
    grouping = sum(1 for a in ledger.values() if a["group"] == "money" and not a["allows_postings"])

    rows_by_status, rows_by_kind = {}, {}
    for row in index_rows:
        bump(rows_by_status, ledger[row]["status"])
        bump(rows_by_kind, ledger[row]["account_kind"])
    holding_rows = sum(1 for row in index_rows if ledger[row]["group"] == "holding")

    holdings_per_brokerage = {}
    holdings_outside = {}
    for account_id, a in ledger.items():
        if a["group"] != "holding":
            continue
        owner = owners[account_id]
        if ledger[owner]["account_kind"] == "brokerage":
            holdings_per_brokerage.setdefault(owner, {"active": 0, "not_active": 0})
            holdings_per_brokerage[owner]["active" if a["status"] == "active" else "not_active"] += 1
        else:
            bump(holdings_outside, a["status"])
    brokerages = [row for row in index_rows if ledger[row]["account_kind"] == "brokerage"]
    snapshot["accounts"] = {
        "by_group_and_status": by_group,
        "postable_by_class_kind_status": postable,
        "non_postable_money_accounts": grouping,
        "account_index_rows": {
            "total": len(index_rows),
            "by_status": rows_by_status,
            "by_kind": rows_by_kind,
            "holdings_listed_as_their_own_row": holding_rows,
        },
        "brokerages": len(brokerages),
        # One entry per brokerage, largest first; positions only, no identity.
        "holdings_per_brokerage": sorted(
            ([holdings_per_brokerage.get(b, {"active": 0, "not_active": 0})[k] for k in ("active", "not_active")] for b in brokerages),
            reverse=True,
        ),
        "holdings_outside_a_brokerage": holdings_outside,
    }

    institutions = current_versions(conn, "institution_versions", "institution_id", as_of, ["status", "kind"])
    by_kind = {}
    for version in institutions.values():
        bump(by_kind, version["kind"], version["status"])
    per_institution = {i: 0 for i in institutions}
    without = 0
    for row in index_rows:
        institution = ledger[row]["institution_id"]
        if institution in per_institution:
            per_institution[institution] += 1
        else:
            without += 1
    snapshot["institutions"] = {
        "total": len(institutions),
        "by_kind_and_status": by_kind,
        "account_index_rows_per_institution": sorted(per_institution.values(), reverse=True),
        "account_index_rows_without_institution": without,
    }

    categories = {i: a for i, a in accounts.items() if a["group"] == "category"}

    def depth(account_id):
        level, seen, current = 0, set(), account_id
        while current in categories and current not in seen:
            seen.add(current)
            level += 1
            current = categories[current]["parent_account_id"]
        return level

    by_depth = {}
    for account_id, a in categories.items():
        bump(by_depth, a["account_class"], f"depth_{depth(account_id)}", a["status"])
    snapshot["categories"] = {"total": len(categories), "by_type_depth_status": by_depth}

    tags = {}
    for kind, status in conn.execute("SELECT kind, status FROM tags"):
        bump(tags, kind, status)
    snapshot["tags"] = {"total": sum(sum(s.values()) for s in tags.values()), "by_kind_and_status": tags}

    payees = current_versions(conn, "payee_versions", "payee_id", as_of, ["status"])
    payee_status = {}
    for version in payees.values():
        bump(payee_status, version["status"])
    snapshot["payees"] = {"total": len(payees), "by_status": payee_status}

    instruments = current_versions(conn, "investment_instrument_versions", "instrument_id", as_of, ["status", "instrument_type"])
    commodity_of = dict(conn.execute("SELECT id, commodity_id FROM investment_instruments").fetchall())
    held = {}
    for commodity_id, side in conn.execute(
        """
        SELECT lot.commodity_id, lot.position_side
        FROM investment_lots lot
        JOIN investment_lot_state state ON state.lot_id = lot.id
        WHERE state.status = 'open' AND state.remaining_quantity_value <> '0'
        """
    ):
        held.setdefault(commodity_id, set()).add(side)
    instrument_counts = {}
    for instrument_id, version in instruments.items():
        sides = held.get(commodity_of.get(instrument_id), set())
        holding = "held_short" if sides == {"short"} else "held" if sides else "not_held"
        bump(instrument_counts, holding, version["status"])
    by_type = {}
    for version in instruments.values():
        bump(by_type, version["instrument_type"])
    snapshot["instruments"] = {"total": len(instruments), "by_holding_and_status": instrument_counts, "by_type": by_type}

    postings = {}
    transactions = set()
    for account_id, transaction_id in conn.execute(
        """
        SELECT pv.account_id, tv.transaction_id
        FROM current_transaction_versions tv
        JOIN transactions t ON t.id = tv.transaction_id
        JOIN journal_entries je ON je.transaction_version_id = tv.id
        JOIN posting_versions pv ON pv.journal_entry_id = je.id
        WHERE tv.status = 'posted' AND t.deleted_at IS NULL
          AND je.entry_date BETWEEN ? AND ?
        """,
        (window_start, window_end),
    ):
        postings[account_id] = postings.get(account_id, 0) + 1
        transactions.add(transaction_id)
    per_row = {row: 0 for row in index_rows}
    for account_id, count in postings.items():
        if account_id in owners:
            per_row[owners[account_id]] += count
    # Holdings are never pinned (D7); a brokerage row still carries the
    # postings of the holdings inside it.
    pin_rows = [row for row in index_rows if ledger[row]["status"] == "active" and ledger[row]["group"] != "holding"]
    active_categories = [i for i, a in categories.items() if a["status"] == "active"]
    report["snapshot"] = snapshot
    report["postings_in_window"] = {
        "from": window_start,
        "to": window_end,
        "days": WINDOW_DAYS,
        "transactions": len(transactions),
        "postings": sum(postings.values()),
        "pin_eligible_account_rows": distribution(per_row[row] for row in pin_rows),
        "pin_eligible_account_rows_largest_first": sorted((per_row[row] for row in pin_rows), reverse=True),
        "active_categories": distribution(postings.get(i, 0) for i in active_categories),
    }
    return report


def print_text(report, out):
    def walk(node, indent):
        for key, value in node.items():
            if isinstance(value, dict):
                print(f"{'  ' * indent}{key}:", file=out)
                walk(value, indent + 1)
            else:
                print(f"{'  ' * indent}{key}: {value}", file=out)

    print("Rekenraam book count (counts only; no names or amounts)", file=out)
    walk(report, 0)


def main(argv=None):
    parser = argparse.ArgumentParser(description="Count a Rekenraam backup copy for the D11 review (#449).")
    parser.add_argument("database", help="a COPY of a backup file, never the live database")
    parser.add_argument(
        "--window-end", help="last day of the 90-day posting window (default: the owner's today); moves nothing else"
    )
    parser.add_argument("--json", action="store_true", help="print JSON instead of text")
    args = parser.parse_args(argv)
    # Dates are compared as text in SQLite, so only the canonical form will do:
    # fromisoformat alone also accepts 20260319.
    if args.window_end is not None:
        if not canonical_day(args.window_end):
            parser.error("--window-end must be a YYYY-MM-DD date")
        if dt.date.fromisoformat(args.window_end) < EARLIEST_WINDOW_END:
            parser.error(f"--window-end must be on or after {EARLIEST_WINDOW_END.isoformat()} so the 90-day window fits")
    try:
        conn = open_copy(args.database)
    except Refused as err:
        print(f"refused: {err}", file=sys.stderr)
        return 2
    try:
        today = owner_today(conn)
        report = count_book(conn, today, args.window_end or today)
    except Refused as err:
        print(f"refused: {err}", file=sys.stderr)
        return 2
    finally:
        conn.close()
    if args.json:
        json.dump(report, sys.stdout, indent=2)
        print()
    else:
        print_text(report, sys.stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main())
