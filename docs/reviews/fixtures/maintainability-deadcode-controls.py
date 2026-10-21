#!/usr/bin/env python3
"""Prepare/verify deliberate scanner controls; no application source changes.

Run the pinned scanners as shown in the report between prepare and verify.
"""
import argparse
import csv
import json
from pathlib import Path

GO = '''package main
type Runner interface { Run() }
type implementation struct{}
func (implementation) Run() { identity(1) }
type RunOptions struct { Enabled bool }
func identity[T any](value T) T { return value }
func invoke(r Runner, options RunOptions) { if options.Enabled { r.Run() } }
func dead() {}
func onlyTest() {}
func main() { invoke(implementation{}, RunOptions{Enabled: true}) }
// dead() is just a comment, not a reachable call.
var text = "dead()"
'''
TEST = '''package main
import "testing"
func TestOnly(t *testing.T) { onlyTest() }
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("phase", choices=["prepare", "verify"])
    parser.add_argument("root", type=Path)
    parser.add_argument("--frontend-deps", type=Path)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    root = args.root
    if args.phase == "prepare":
        root.mkdir(parents=True, exist_ok=False)
        (root / "backend").mkdir()
        (root / "backend/go.mod").write_text("module controls\n\ngo 1.27.0\n")
        (root / "backend/main.go").write_text(GO)
        (root / "backend/main_test.go").write_text(TEST)
        for path in ("src/routes", "src/lib"):
            (root / "web" / path).mkdir(parents=True)
        assert args.frontend_deps
        (root / "web/node_modules").symlink_to(args.frontend_deps.resolve(), target_is_directory=True)
        (root / "web/package.json").write_text(json.dumps({"private": True, "type": "module", "dependencies": {"svelte": "5.57.2", "@sveltejs/kit": "3.1.0"}}))
        (root / "web/vite.config.ts").write_text("import {sveltekit} from '@sveltejs/kit/vite';\nexport default {plugins:[sveltekit()]};\n")
        (root / "web/tsconfig.json").write_text(json.dumps({"compilerOptions": {"moduleResolution": "Bundler", "module": "ESNext"}}))
        (root / "web/src/routes/+page.svelte").write_text('<script lang="ts">import {live} from "../lib/helper.ts";</script><p>{live()}</p>\n')
        (root / "web/src/lib/helper.ts").write_text("export function live(){return localOnly()}\nexport function localOnly(){return 1}\nexport function unreachable(){return 2}\nexport type UnusedShape={value:number};\n")
        (root / "web/src/lib/orphan.ts").write_text("export const orphan = 1;\n")
        (root / "web/src/lib/orphan.svelte").write_text("<p>Unreachable control</p>\n")
        config = {"project": ["src/**/*.{ts,svelte}"], "entry": ["src/routes/**/+*.svelte", "vite.config.ts"]}
        (root / "web/knip.json").write_text(json.dumps(config))
        config["ignoreExportsUsedInFile"] = True
        (root / "web/knip.local.json").write_text(json.dumps(config))
        print(root)
        return
    def functions(name):
        return sorted(f["Name"] for pkg in json.loads((root / name).read_text()) for f in pkg["Funcs"])
    assert functions("go-prod.json") == ["dead", "onlyTest"]
    assert functions("go-tests.json") == ["dead"]
    rows = list(csv.DictReader((root / "go-types.csv").open()))
    assert sorted(r["type"] for r in rows if r["kind"] == "definition") == ["RunOptions", "Runner"]
    assert {r["expression"] for r in rows if r["kind"] == "member-reference"} == {"Run", "Enabled"}
    def web(name):
        report = json.loads((root / name).read_text())
        return {kind: sorted((issue["file"], row["name"]) for issue in report["issues"] for row in issue.get(kind, []))
                for kind in ("files", "exports", "types", "unresolved")}
    raw, local = web("web.json"), web("web-local.json")
    assert [name for _, name in raw["files"]] == ["src/lib/orphan.svelte", "src/lib/orphan.ts"]
    assert [name for _, name in raw["exports"]] == ["localOnly", "unreachable"]
    assert [name for _, name in local["exports"]] == ["unreachable"]
    assert [name for _, name in raw["types"]] == ["UnusedShape"]
    assert not raw["unresolved"] and not local["unresolved"]
    result = {"go_dynamic_interface_live": True, "go_test_root_difference": True,
              "go_comment_string_negative": True, "named_types_exclude_generic_parameters": True,
              "web_route_and_imported_helper_live": True, "web_unused_ts_and_svelte_files": True,
              "web_local_export_visibility_distinguished": True, "web_unused_type": True}
    assert args.output
    args.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    print("All dead-code and abstraction scanner controls passed")


if __name__ == "__main__":
    main()
