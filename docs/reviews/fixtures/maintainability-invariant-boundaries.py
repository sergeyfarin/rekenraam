#!/usr/bin/env python3
"""Index selected frontend call-shaped sites and SQL Up trigger declarations.

Reads Git objects at one ref. Neither index is semantic enforcement analysis;
SQL replacements are retained, rather than simulated as the final schema.
"""

import argparse
import csv
from pathlib import Path
import re
import subprocess


def git(*args):
    return subprocess.check_output(["git", *args], text=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ref", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    ref = git("rev-parse", "--verify", args.ref + "^{commit}").strip()
    args.output.mkdir(parents=True, exist_ok=True)
    paths = git("ls-tree", "-r", "--name-only", ref).splitlines()
    calls = re.compile(r"\b(checkpointOverride|gainAcknowledgement|isStaleImpactRefusal|impactNeedsReview|useOwnerToday|coefficientSign|parseMagnitude|parseAmountField)\s*\(")
    triggers = re.compile(r"^CREATE TRIGGER(?: IF NOT EXISTS)?\s+(\w+)", re.M)
    client, sql = [], []
    for path in paths:
        if (path.startswith("frontend/src/") and path.endswith((".ts", ".svelte"))
                and not path.endswith((".test.ts", "/schema.d.ts")) and "/paraglide/" not in path):
            for line, source in enumerate(git("show", f"{ref}:{path}").splitlines(), 1):
                for match in calls.finditer(source):
                    client.append([path, line, match[1], "lexical-call", source.strip()])
        if path.startswith("backend/migrations/") and path.endswith(".sql"):
            up = git("show", f"{ref}:{path}").split("-- +goose Down", 1)[0]
            for match in triggers.finditer(up):
                sql.append([path, up[:match.start()].count("\n") + 1, match[1], "up-declaration"])
    for kind, header, rows in (
        ("client", ["file", "line", "symbol", "kind", "source"], client),
        ("sql", ["file", "line", "trigger", "kind"], sql),
    ):
        with (args.output / f"{kind}.csv").open("w", newline="") as output:
            writer = csv.writer(output, lineterminator="\n")
            writer.writerow(header)
            writer.writerows(rows)
        print(f"{kind}: {len(rows)} sites in {len({row[0] for row in rows})} files at {ref}")


if __name__ == "__main__":
    main()
