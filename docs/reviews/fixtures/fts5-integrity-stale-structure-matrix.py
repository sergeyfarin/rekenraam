#!/usr/bin/env python3
"""#541 probe matrix: PROBE OUTPUT.json NAME=AMALGAMATION_DIR [NAME=DIR ...]

Builds fts5-integrity-stale-structure-probe.c.txt against each official
amalgamation, and against a copy of each with the candidate engine patch
(fts5NewTransaction() before the check in fts5IntegrityMethod). It then runs:

- the base probe in both journal modes, with and without `refresh`;
- invalid arguments, which must exit 2;
- source mutations of the probe. Most break one control, opening step or
  statement, and must exit 2. Two change one damage-control expected text,
  standing in for a reworded engine message, and must exit 2 through that
  branch. One skips the FTS5 command, so the reader is stale at the damage
  check, and must match the base probe.

Every case also names a line its stdout must contain, which shows which
branch decided the run. OUTPUT.json holds every run (expected and actual
exit status, required line, full stdout) and the input hashes, and is
deterministic for the same inputs. The volatile environment (time, compiler,
platform) goes to OUTPUT.environment.json. The script exits 1 if any run
differs from its expectation. Run it with `python3 -I`. It
needs `cc`, and the amalgamations must be authenticated beforehand (see the
review). Each run uses its own empty directory under a temporary root.
"""
import datetime
import hashlib
import json
from pathlib import Path
import platform
import subprocess
import sys
import tempfile

CFLAGS = ["-O1", "-DSQLITE_THREADSAFE=1", "-DSQLITE_ENABLE_FTS5"]
LIBS = ["-lm", "-lpthread", "-ldl"]

PATCH_OLD = "  rc = sqlite3Fts5StorageIntegrity(pTab->pStorage, 0);\n"
PATCH_NEW = ("  rc = fts5NewTransaction(pTab);\n"
             "  if( rc==SQLITE_OK ) rc = sqlite3Fts5StorageIntegrity(pTab->pStorage, 0);\n")

DENY = ("static int deny_on;\n"
        "static int deny(void*u,int op,const char*a,const char*b,const char*c,const char*d){"
        "(void)u;(void)a;(void)b;(void)c;(void)d;"
        "return deny_on&&op==SQLITE_PRAGMA?SQLITE_DENY:SQLITE_OK;}\n")
OPEN_READER = 'sqlite3 *r = open_db("open reader");'
READER_DAMAGE = ('    if (reader_check(r, refresh, got, sizeof got) == SQLITE_OK) {\n'
                 '      printf("reader integrity_check after')

FRESH_DAMAGE = 'else if (!strstr(got, "malformed inverted index for FTS5 table main.t"))'
READER_DAMAGE_MATCH = '\n      if (!strstr(got, "malformed inverted index for FTS5 table main.t"))'
QUOTED_TABLE = '\\"main\\".\\"t\\"'  # C source text for "main"."t"
STALE_READER_DAMAGE = ("reader integrity_check after deleting a t_docsize row: "
                       "malformed inverted index for FTS5 table main.t")

# name -> (list of (exact text in the probe, replacement),
#          expected exit: 2, or None for "same as the base probe",
#          required stdout text proving which branch decided the run: a string,
#          or a function of "does the base probe show the defect" -> list)
MUTATIONS = {
    "authorizer denies the reader pragma at the damage check": ([
        ("static int invalid;", DENY + "static int invalid;"),
        (OPEN_READER, OPEN_READER + " sqlite3_set_authorizer(r, deny, 0);"),
        (READER_DAMAGE, "    deny_on = 1;\n" + READER_DAMAGE),
    ], 2, "INVALID: reader PRAGMA integrity_check: prepare: not authorized"),
    "authorizer denies every reader pragma": ([
        ("static int invalid;", DENY + "static int invalid;"),
        (OPEN_READER, OPEN_READER + " sqlite3_set_authorizer(r, deny, 0); deny_on = 1;"),
    ], 2, "INVALID: reader PRAGMA integrity_check: prepare: not authorized"),
    "fresh-connection control returns not ok": ([
        ('int rc = query(f, "PRAGMA integrity_check"', "int rc = query(f, \"SELECT 'not ok'\""),
    ], 2, "INVALID: fresh-connection control: not ok"),
    "FTS5 integrity-check command fails": ([
        ("VALUES('integrity-check')\", 0", "VALUES('integrity-chek')\", 0"),
    ], 2, "INVALID: FTS5 'integrity-check' control"),
    "damage DELETE matches no row": ([
        ("t_docsize WHERE id = 2", "t_docsize WHERE id = 99"),
    ], 2, "INVALID: damage control: no t_docsize row deleted"),
    "damage DELETE fails": ([
        ("DELETE FROM t_docsize WHERE id = 2", "DELETE FROM t_docsizzz WHERE id = 2"),
    ], 2, "INVALID: DELETE FROM t_docsizzz WHERE id = 2: no such table"),
    "writer INSERT between checks fails": ([
        ("INSERT INTO t(x) VALUES('gamma delta %d')", "INSERT INTO nope(x) VALUES('gamma delta %d')"),
    ], 2, "INVALID: INSERT INTO nope(x)"),
    "fresh connection cannot open": ([
        ('SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE, 0);',
         'SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE,'
         ' strcmp(who, "open fresh connection") ? 0 : "no-such-vfs");'),
    ], 2, "INVALID: open fresh connection: no such vfs"),
    # These change the probe's expected text, not the engine's message: they
    # stand in for an engine that words its damage finding differently, and
    # take the fresh and reader exact-match branches one at a time.
    "fresh damage-control expected text changed (simulated)": ([
        (FRESH_DAMAGE, FRESH_DAMAGE.replace("main.t", QUOTED_TABLE)),
    ], 2, "INVALID: fresh-connection damage control: no FTS5 corruption finding for t"),
    "reader damage-control expected text changed (simulated)": ([
        (READER_DAMAGE_MATCH, READER_DAMAGE_MATCH.replace("main.t", QUOTED_TABLE)),
    ], 2, "INVALID: reader damage control: no FTS5 corruption finding for t"),
    # The FTS5 command is replaced by a bare reader integrity_check. It prints
    # its result, and its return code still feeds the FTS5-command control.
    # On the unpatched engines it prints checksum mismatch: the reader is stale
    # just before the damage check. Without `refresh`, the damage check runs
    # in autocommit and the reader stays stale through that pragma. FTS5 checks
    # docsize before it compares checksums, so the stale reader still reports
    # the corruption itself, and the exit status matches the base probe. With
    # `refresh`, reader_check refreshes first, so those rows only show that
    # the bare check was stale beforehand.
    "FTS5 command replaced by a bare reader check (reader stale just before the damage check)": ([
        ("  rc = sqlite3_exec(r, \"INSERT INTO t(t) VALUES('integrity-check')\", 0, 0, 0);\n"
         "  printf(\"reader FTS5 'integrity-check' command: %s\\n\", rc == SQLITE_OK ? \"ok\" : sqlite3_errmsg(r));",
         "  rc = query(r, \"PRAGMA integrity_check\", got, sizeof got);\n"
         "  printf(\"reader FTS5 'integrity-check' command: replaced by a bare reader check (mutation)\\n\");\n"
         "  printf(\"reader bare integrity_check before damage: %s\\n\", got);"),
    ], None, lambda defect: (
        ['reader bare integrity_check before damage: fts5: checksum mismatch for table "t"',
         STALE_READER_DAMAGE] if defect else [STALE_READER_DAMAGE])),
}

BAD_ARGS = [["wal", "refesh"], ["wal", "refresh", "extra"], ["wl"], ["wal", ""]]


def replace_once(text, old, new, what):
    if text.count(old) != 1:
        raise SystemExit(f"{what}: expected one occurrence of {old[:60]!r}")
    return text.replace(old, new)


def sha3(path):
    return hashlib.sha3_256(Path(path).read_bytes()).hexdigest()


def sha256(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def parse_engines(args):
    engines, names = [], set()
    for arg in args:
        name, sep, directory = arg.partition("=")
        if not sep or not name or not directory:
            raise SystemExit(f"expected NAME=AMALGAMATION_DIR, got {arg!r}")
        if name in names or "/" in name or name.endswith("+patch"):
            raise SystemExit(f"engine name {name!r} is repeated, contains '/' or ends in '+patch'")
        path = Path(directory).resolve()
        for required in ("sqlite3.c", "sqlite3.h"):
            if not (path / required).is_file():
                raise SystemExit(f"{name}: {path / required} is missing")
        names.add(name)
        engines.append((name, path))
    return engines


def compile_engine(source_dir, work, name):
    obj = work / f"sqlite3-{name}.o"
    subprocess.run(["cc", *CFLAGS, "-I", str(source_dir), "-c", str(source_dir / "sqlite3.c"),
                    "-o", str(obj)], check=True)
    return obj


def link_probe(source, include_dir, engine_obj, out):
    c_file = out.with_name(out.name + ".c")  # names contain dots: no with_suffix
    c_file.write_text(source)
    subprocess.run(["cc", *CFLAGS, "-Wall", "-Wextra", "-Werror", "-I", str(include_dir),
                    str(c_file), str(engine_obj), "-o", str(out), *LIBS], check=True)
    return out


def run(binary, args, root):
    cwd = Path(tempfile.mkdtemp(dir=root))
    proc = subprocess.run([str(binary), *args], cwd=cwd, capture_output=True, text=True, timeout=60)
    # The usage line echoes argv[0]; keep machine-local paths out of the evidence.
    lines = proc.stdout.replace(str(root), "<work>").splitlines()
    return {
        "exit": proc.returncode,
        "stdout": lines,
        "verdict": lines[-1] if lines else "",
        "invalid": next((line for line in lines if line.startswith("INVALID:")), None),
        "engine_version": lines[0] if lines and lines[0].startswith("sqlite ") else None,
    }


def main():
    if len(sys.argv) < 4:
        raise SystemExit(__doc__)
    probe = Path(sys.argv[1]).resolve()
    output = Path(sys.argv[2]).resolve()
    engines = parse_engines(sys.argv[3:])
    probe_source = probe.read_text()
    engine_provenance = {}

    runs = []
    failed = False
    with tempfile.TemporaryDirectory() as tmp:
        work = Path(tmp)
        built = []
        for name, directory in engines:
            official = sha3(directory / "sqlite3.c")
            engine_provenance[name] = {"official_sqlite3_c_sha3_256": official}
            built.append((name, directory, official, False))
            patched_dir = work / f"{name}+patch"
            patched_dir.mkdir()
            (patched_dir / "sqlite3.h").write_bytes((directory / "sqlite3.h").read_bytes())
            amalgamation = (directory / "sqlite3.c").read_text(errors="surrogateescape")
            (patched_dir / "sqlite3.c").write_text(
                replace_once(amalgamation, PATCH_OLD, PATCH_NEW, f"{name} patch"), errors="surrogateescape")
            engine_provenance[f"{name}+patch"] = {
                "official_sqlite3_c_sha3_256": official,
                "patched_sqlite3_c_sha3_256": sha3(patched_dir / "sqlite3.c"),
            }
            built.append((f"{name}+patch", patched_dir, official, True))

        for name, directory, source_sha3, patched in built:
            print(name, flush=True)
            engine = compile_engine(directory, work, name)
            base = link_probe(probe_source, directory, engine, work / f"probe-{name}")
            def base_exit(args):
                return 0 if patched or "refresh" in args else 1

            def base_line(args):
                if base_exit(args):
                    return 'reader integrity_check run 2: fts5: checksum mismatch for table "t"'
                return "reader integrity_check run 2: ok"

            cases = []
            for mode in ("wal", "delete"):
                for args in ([mode], [mode, "refresh"]):
                    cases.append(("base", args, base_exit(args), [base_line(args)], base))
            for args in BAD_ARGS:
                cases.append(("invalid arguments", args, 2, ["usage: <work>/probe-"], base))
            for index, (mutation, (replacements, expected, line)) in enumerate(MUTATIONS.items()):
                source = probe_source
                for old, new in replacements:
                    source = replace_once(source, old, new, mutation)
                binary = link_probe(source, directory, engine, work / f"probe-{name}-m{index}")
                for args in (["wal"], ["wal", "refresh"]):
                    lines = line(not patched) if callable(line) else [line]
                    cases.append((mutation, args, base_exit(args) if expected is None else expected,
                                  lines, binary))
            for case, args, expected, lines, binary in cases:
                result = run(binary, args, work)
                ok = result["exit"] == expected and all(
                    any(want in out for out in result["stdout"]) for want in lines)
                failed |= not ok
                runs.append({"engine": name, "official_sqlite3_c_sha3_256": source_sha3,
                             "candidate_patch": patched, "case": case, "args": args,
                             "expected_exit": expected, "required_lines": lines, **result,
                             "as_expected": ok})
                print(f"  {'ok ' if ok else 'BAD'} {case} {args} exit={result['exit']}", flush=True)

    compiler = subprocess.run(["cc", "--version"], capture_output=True, text=True, check=True)
    output.with_name(output.stem + ".environment.json").write_text(json.dumps({
        "generated_at": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
        "compiler": compiler.stdout.splitlines()[0],
        "platform": platform.platform(),
    }, indent=2) + "\n")
    output.write_text(json.dumps({
        "probe_sha256": sha256(probe),
        "matrix_sha256": sha256(__file__),
        "cflags": CFLAGS,
        "engines": engine_provenance,
        "runs": runs,
        "all_as_expected": not failed,
    }, indent=2) + "\n")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
