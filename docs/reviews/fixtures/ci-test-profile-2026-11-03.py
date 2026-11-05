#!/usr/bin/env python3
"""Collect the CI and browser timing evidence used by the 2026-11-03 profile.

The script only reads GitHub Actions metadata through the authenticated `gh`
CLI, plus already-downloaded E2E timing artifacts. It does not run a workflow,
change a cache or write outside the requested output file.

Examples:

    # Recent successful CI runs on main, with job/step medians.
    python3 docs/reviews/fixtures/ci-test-profile-2026-11-03.py \
        --runs 8 --output /tmp/rekenraam-ci.json

    # Exact runs plus extracted browser-timing artifact directories.
    # Download each artifact with:
    #   gh api repos/OWNER/REPO/actions/artifacts/ARTIFACT_ID/zip > name.zip
    #   python3 -m zipfile -e name.zip DIRECTORY
    python3 docs/reviews/fixtures/ci-test-profile-2026-11-03.py \
        --run-id 41530724953 \
        --browser-dir /tmp/browser-timings-app \
        --browser-dir /tmp/browser-timings-audits \
        --output /tmp/rekenraam-ci.json

`summary.json` and `attempts.ndjson` are the files written by the E2E timing
reporter. Case durations and API-action durations overlap; do not add them
together as wall time.
"""

import argparse
import collections
import datetime
import json
import statistics
import subprocess
from pathlib import Path


def gh_json(path):
    output = subprocess.check_output(["gh", "api", path], text=True)
    return json.loads(output)


def timestamp(value):
    return datetime.datetime.fromisoformat(value.replace("Z", "+00:00"))


def seconds(start, end):
    return round((timestamp(end) - timestamp(start)).total_seconds(), 1)


def ci_runs(repo, count, run_ids):
    if run_ids:
        selected = []
        for run_id in run_ids:
            selected.append(gh_json(f"repos/{repo}/actions/runs/{run_id}"))
    else:
        response = gh_json(
            f"repos/{repo}/actions/runs?branch=main&status=success&per_page=50"
        )
        selected = [
            run
            for run in response["workflow_runs"]
            if run["name"] == "CI" and run["event"] == "push"
        ][:count]

    runs = []
    job_values = collections.defaultdict(list)
    step_values = collections.defaultdict(list)
    for run in selected:
        response = gh_json(f"repos/{repo}/actions/runs/{run['id']}/jobs?per_page=100")
        entry = {
            "id": run["id"],
            "head_sha": run["head_sha"],
            "event": run["event"],
            "created_at": run["created_at"],
            "updated_at": run["updated_at"],
            "wall_seconds": seconds(run["created_at"], run["updated_at"]),
            "jobs": [],
        }
        for job in response["jobs"]:
            job_entry = {
                "name": job["name"],
                "conclusion": job["conclusion"],
                "wall_seconds": seconds(job["started_at"], job["completed_at"]),
                "steps": [],
            }
            job_values[job["name"]].append(job_entry["wall_seconds"])
            for step in job.get("steps", []):
                if not step.get("started_at") or not step.get("completed_at"):
                    continue
                step_entry = {
                    "name": step["name"],
                    "seconds": seconds(step["started_at"], step["completed_at"]),
                    "conclusion": step["conclusion"],
                }
                job_entry["steps"].append(step_entry)
                step_values[(job["name"], step["name"])].append(step_entry["seconds"])
            entry["jobs"].append(job_entry)
        runs.append(entry)

    return {
        "runs": runs,
        "job_medians": {
            name: {
                "median": round(statistics.median(values), 1),
                "min": min(values),
                "max": max(values),
                "runs": len(values),
            }
            for name, values in sorted(job_values.items())
        },
        "step_medians": [
            {
                "job": job,
                "step": step,
                "median": round(statistics.median(values), 1),
                "min": min(values),
                "max": max(values),
                "runs": len(values),
            }
            for (job, step), values in sorted(
                step_values.items(), key=lambda item: -statistics.median(item[1])
            )
        ],
    }


def browser_summary(directory):
    directory = Path(directory)
    summary = json.loads((directory / "summary.json").read_text())
    attempts = [
        json.loads(line)
        for line in (directory / "attempts.ndjson").read_text().splitlines()
        if line.strip()
    ]
    projects = collections.defaultdict(list)
    actions = collections.defaultdict(float)
    steps = collections.defaultdict(float)
    for attempt in attempts:
        projects[attempt["project"]].append(attempt["duration_ms"] / 1000)
        for name, action in (attempt.get("api") or {}).items():
            actions[name] += action.get("duration_ms", 0) / 1000
        for step in attempt.get("steps") or []:
            steps[step["category"]] += step.get("duration_ms", 0) / 1000

    return {
        "source": summary.get("source"),
        "status": summary.get("status"),
        "duration_s": round(summary["duration_ms"] / 1000, 1),
        "setup_s": round(summary["setup"]["duration_ms"] / 1000, 1),
        "build_s": round(summary["setup"].get("build_ms", 0) / 1000, 1),
        "workers": summary.get("workers"),
        "runner": summary.get("runner"),
        "selected": len(summary.get("selected", [])),
        "attempts": len(attempts),
        "projects": {
            name: {
                "attempts": len(values),
                "case_seconds": round(sum(values), 1),
                "median_s": round(statistics.median(values), 2),
                "max_s": round(max(values), 1),
            }
            for name, values in sorted(
                projects.items(), key=lambda item: -sum(item[1])
            )
        },
        "slowest_attempts": [
            {
                "project": attempt["project"],
                "title": attempt["title"][-1],
                "seconds": round(attempt["duration_ms"] / 1000, 1),
            }
            for attempt in sorted(
                attempts, key=lambda item: -item["duration_ms"]
            )[:12]
        ],
        "api_action_seconds": {
            name: round(value, 1)
            for name, value in sorted(actions.items(), key=lambda item: -item[1])[:12]
        },
        "step_category_seconds": {
            name: round(value, 1)
            for name, value in sorted(steps.items(), key=lambda item: -item[1])[:12]
        },
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", default="sergeyfarin/rekenraam")
    parser.add_argument("--runs", type=int, default=8)
    parser.add_argument("--run-id", action="append", type=int, default=[])
    parser.add_argument("--browser-dir", action="append", default=[])
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    result = {
        "schema": 1,
        "collected_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "repo": args.repo,
        "ci": ci_runs(args.repo, args.runs, args.run_id),
        "browser_timings": {
            Path(directory).name: browser_summary(directory)
            for directory in args.browser_dir
        },
        "method": {
            "ci": "successful CI push runs on main through the GitHub Actions API",
            "browser": "E2E timing-reporter summary.json and attempts.ndjson",
            "overlap": "case, step and API-action durations overlap; they are not additive",
        },
    }
    Path(args.output).write_text(json.dumps(result, indent=2) + "\n")
    ci = result["ci"]
    print(
        json.dumps(
            {
                "runs": len(ci["runs"]),
                "browser_suites": list(result["browser_timings"]),
                "slowest_step_medians": ci["step_medians"][:6],
            },
            indent=2,
        )
    )


if __name__ == "__main__":
    main()
