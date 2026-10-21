#!/usr/bin/env python3
"""Prepare fixed-SHA runtime inputs for #284's dupl and jscpd scans.

Svelte script/markup projections preserve line numbers. They are lexical scan
inputs, not Svelte compiler output. Manifests record every input and its hash.
"""

import argparse
import hashlib
import io
import json
from pathlib import Path
import re
import subprocess
import tarfile


def mask(text):
    return "".join("\n" if c == "\n" else " " for c in text)


def project_svelte(text):
    # Top-level Svelte script/style tags in this baseline start on their own
    # line. Refuse unmatched tags rather than claiming coverage of a partial file.
    attributes = r'''(?:[^"'<>]|"[^"]*"|'[^']*')*'''
    pattern = r"^<(script|style)\b" + attributes + r">[\s\S]*?^</\1\s*>"
    blocks = list(re.finditer(pattern, text, re.M))
    openings = list(re.finditer(r"^<(script|style)\b", text, re.M))
    assert len(blocks) == len(openings), "unmatched script/style block"
    scripts = []
    markup = text
    for block in reversed(blocks):
        markup = markup[:block.start()] + mask(block.group()) + markup[block.end():]
    for block in blocks:
        if block.group(1) != "script":
            continue
        opening = re.match(r"^<script\b" + attributes + r">", block.group())
        start = block.start() + opening.end()
        end = block.start() + block.group().rindex("</script")
        projected = mask(text[:start]) + text[start:end] + mask(text[end:])
        assert projected.count("\n") == text.count("\n")
        scripts.append(projected)
    assert markup.count("\n") == text.count("\n")
    return scripts, markup


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ref", required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    ref = subprocess.check_output(["git", "rev-parse", "--verify", args.ref + "^{commit}"], text=True).strip()
    args.output.mkdir(parents=True, exist_ok=False)
    archive = subprocess.check_output(["git", "archive", ref, "backend", "frontend/src"])
    manifest = dict(ref=ref, go=[], frontend=[])
    with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
        for member in tar:
            path = member.name
            go = path.startswith("backend/") and path.endswith(".go") and not path.endswith("_test.go")
            frontend = (path.startswith("frontend/src/") and path.endswith((".ts", ".svelte"))
                        and not path.endswith((".test.ts", "/schema.d.ts")) and "/paraglide/" not in path)
            if not member.isfile() or not (go or frontend):
                continue
            content = tar.extractfile(member).read()
            dest = args.output / "source" / path
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_bytes(content)
            entry = dict(path=path, sha256=hashlib.sha256(content).hexdigest(), projections=[])
            if go:
                manifest["go"].append(entry)
                continue
            text = content.decode()
            if path.endswith(".svelte"):
                scripts, markup = project_svelte(text)
                projections = [(path + f".script{i}.ts", source) for i, source in enumerate(scripts, 1)]
                projections.append((path + ".markup.html", markup))
            else:
                projections = [(path, text)]
            for projected_path, source in projections:
                projected = args.output / "frontend" / projected_path
                projected.parent.mkdir(parents=True, exist_ok=True)
                projected.write_text(source)
                entry["projections"].append(dict(path=projected_path,
                    sha256=hashlib.sha256(source.encode()).hexdigest()))
            manifest["frontend"].append(entry)
    for language in ("go", "frontend"):
        manifest[language].sort(key=lambda entry: entry["path"])
    (args.output / "go-files.txt").write_text("".join(entry["path"] + "\n" for entry in manifest["go"]))
    (args.output / "manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
    print(json.dumps(dict(ref=ref, go_files=len(manifest["go"]),
                         frontend_files=len(manifest["frontend"]),
                         frontend_projections=sum(len(entry["projections"]) for entry in manifest["frontend"]))))


if __name__ == "__main__":
    main()
