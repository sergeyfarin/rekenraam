#!/usr/bin/env python3
"""Reproduce #284's history baseline without dependencies or working-tree reads."""

import argparse
from collections import Counter
import csv
from itertools import combinations
import json
import re
import subprocess
import sys


def git(*args):
    return subprocess.check_output(["git", *args]).decode("utf-8")


def source_kind(path):
    if path.startswith("backend/") and path.endswith(".go"):
        return "test" if path.endswith("_test.go") else "backend"
    if path.startswith("frontend/src/") and path.endswith((".ts", ".svelte", ".css")):
        if "/paraglide/" in path or path.endswith("/schema.d.ts"):
            return None
        return "test" if path.endswith(".test.ts") else "frontend"
    if path.startswith("e2e/") and path.endswith(".ts"):
        return "test"
    if path.startswith("api/openapi/") and path.endswith(".yaml"):
        return "contract"
    return None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ref", required=True)
    parser.add_argument("--since", default="2026-10-01T00:00:00Z")
    parser.add_argument("--format", choices=("json", "csv"), default="json")
    args = parser.parse_args()
    ref = git("rev-parse", "--verify", args.ref + "^{commit}").strip()
    paths = git("ls-tree", "-r", "--name-only", ref).splitlines()
    rows = {}
    for path in paths:
        kind = source_kind(path)
        if kind:
            rows[path] = dict(path=path, kind=kind, lines=0, added=0, deleted=0,
                              touches=0, fixes=0, features=0)
    # One batch avoids launching a process per file. Git writes one size header
    # followed by the exact blob bytes and a newline for each requested object.
    blob_paths = sorted(rows)
    blobs = subprocess.check_output(
        ["git", "cat-file", "--batch"],
        input="".join(ref + ":" + path + "\n" for path in blob_paths).encode(),
    )
    offset = 0
    for path in blob_paths:
        header_end = blobs.index(b"\n", offset)
        size = int(blobs[offset:header_end].split()[-1])
        start = header_end + 1
        rows[path]["lines"] = len(blobs[start:start + size].splitlines())
        offset = start + size + 1
    assert offset == len(blobs)
    history = git("log", ref, "--no-merges", "--since-as-filter=" + args.since,
                  "--format=COMMIT%x09%H%x09%cI%x09%s", "--numstat", "--no-renames")
    commits = []
    current = None
    for line in history.splitlines():
        if line.startswith("COMMIT\t"):
            _, sha, date, subject = line.split("\t", 3)
            current = dict(sha=sha, date=date, subject=subject, files=[])
            commits.append(current)
        elif line and current is not None:
            added, deleted, path = line.split("\t", 2)
            if path in rows and added != "-" and deleted != "-":
                row = rows[path]
                row["added"] += int(added)
                row["deleted"] += int(deleted)
                row["touches"] += 1
                row["fixes"] += bool(re.match(r"^fix(?:\([^)]*\))?!?:", current["subject"]))
                row["features"] += bool(re.match(r"^feat(?:\([^)]*\))?!?:", current["subject"]))
                current["files"].append(path)
    pairs = Counter()
    features = []
    for commit in commits:
        if not re.match(r"^feat(?:\([^)]*\))?!?:", commit["subject"]):
            continue
        runtime = sorted(p for p in commit["files"] if rows[p]["kind"] in ("backend", "frontend"))
        pairs.update(combinations(runtime, 2))
        features.append(dict(sha=commit["sha"], subject=commit["subject"],
                             runtime_files=len(runtime), files=runtime))
    for row in rows.values():
        row["churn"] = row["added"] + row["deleted"]
        row["churn_times_lines"] = row["churn"] * row["lines"]
    output = dict(
        ref=ref, since=args.since, non_merge_commits=len(commits),
        fix_commits=sum(bool(re.match(r"^fix(?:\([^)]*\))?!?:", c["subject"])) for c in commits),
        feature_commits=len(features),
        files=sorted(rows.values(), key=lambda r: (-r["churn_times_lines"], r["path"])),
        feature_changes=sorted(features, key=lambda c: (-c["runtime_files"], c["sha"])),
        feature_cochanges=[dict(files=list(pair), commits=count) for pair, count in
                           sorted(pairs.items(), key=lambda item: (-item[1], item[0]))],
    )
    if args.format == "csv":
        writer = csv.DictWriter(sys.stdout, fieldnames=list(output["files"][0]), lineterminator="\n")
        writer.writeheader()
        writer.writerows(output["files"])
    else:
        print(json.dumps(output, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
