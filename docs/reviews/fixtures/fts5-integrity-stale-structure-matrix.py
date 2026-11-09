#!/usr/bin/env python3
"""#541 probe matrix: PROBE OUTPUT.json NAME=AMALGAMATION_DIR [NAME=DIR ...]

Builds fts5-integrity-stale-structure-probe.c.txt against each official
amalgamation, and against a copy of each with the candidate engine patch
(fts5NewTransaction() before the check in fts5IntegrityMethod). It then runs:

- the base probe in both journal modes, with and without `refresh`;
- invalid arguments, which must exit 2;
- injected failures, which must exit 2: each is a source mutation of the probe
  that breaks one control, opening step or statement, or simulates an engine
  that rewords the damage-control message.

Writes every run (expected and actual exit status, plus the probe's full
stdout) and the provenance of every input to OUTPUT.json, and exits 1 if any
run differs from its expectation. Run it with `python3 -I`. It
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

# name -> list of (exact text in the probe, replacement)
MUTATIONS = {
    "authorizer denies the reader pragma at the damage check": [
        ("static int invalid;", DENY + "static int invalid;"),
        (OPEN_READER, OPEN_READER + " sqlite3_set_authorizer(r, deny, 0);"),
        (READER_DAMAGE, "    deny_on = 1;\n" + READER_DAMAGE),
    ],
    "authorizer denies every reader pragma": [
        ("static int invalid;", DENY + "static int invalid;"),
        (OPEN_READER, OPEN_READER + " sqlite3_set_authorizer(r, deny, 0); deny_on = 1;"),
    ],
    "fresh-connection control returns not ok": [
        ('int rc = query(f, "PRAGMA integrity_check"', "int rc = query(f, \"SELECT 'not ok'\""),
    ],
    "FTS5 integrity-check command fails": [
        ("VALUES('integrity-check')\", 0", "VALUES('integrity-chek')\", 0"),
    ],
    "damage DELETE matches no row": [
        ("t_docsize WHERE id = 2", "t_docsize WHERE id = 99"),
    ],
    "damage DELETE fails": [
        ("DELETE FROM t_docsize WHERE id = 2", "DELETE FROM t_docsizzz WHERE id = 2"),
    ],
    "writer INSERT between checks fails": [
        ("INSERT INTO t(x) VALUES('gamma delta %d')", "INSERT INTO nope(x) VALUES('gamma delta %d')"),
    ],
    # A future engine might quote the table differently; the probe must then
    # refuse to interpret the run rather than pass or fail it.
    "damage-control message reworded (simulated)": [
        ('else if (!strstr(got, "malformed inverted index for FTS5 table main.t"))',
         'else if (!strstr(got, "malformed inverted index for FTS5 table \\"main\\".\\"t\\""))'),
        ('if (!strstr(got, "malformed inverted index for FTS5 table main.t") &&',
         'if (!strstr(got, "malformed inverted index for FTS5 table \\"main\\".\\"t\\"") &&'),
    ],
    "fresh connection cannot open": [
        ('SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE, 0);',
         'SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE,'
         ' strcmp(who, "open fresh connection") ? 0 : "no-such-vfs");'),
    ],
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
        if name in names or "/" in name:
            raise SystemExit(f"engine name {name!r} is repeated or contains '/'")
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
            cases = []
            for mode in ("wal", "delete"):
                cases.append(("base", [mode], 0 if patched else 1, base))
                cases.append(("base", [mode, "refresh"], 0, base))
            for args in BAD_ARGS:
                cases.append(("invalid arguments", args, 2, base))
            for index, (mutation, replacements) in enumerate(MUTATIONS.items()):
                source = probe_source
                for old, new in replacements:
                    source = replace_once(source, old, new, mutation)
                binary = link_probe(source, directory, engine, work / f"probe-{name}-m{index}")
                for args in (["wal"], ["wal", "refresh"]):
                    cases.append((mutation, args, 2, binary))
            for case, args, expected, binary in cases:
                result = run(binary, args, work)
                ok = result["exit"] == expected
                failed |= not ok
                runs.append({"engine": name, "official_sqlite3_c_sha3_256": source_sha3,
                             "candidate_patch": patched, "case": case, "args": args,
                             "expected_exit": expected, **result, "as_expected": ok})
                print(f"  {'ok ' if ok else 'BAD'} {case} {args} exit={result['exit']}", flush=True)

    compiler = subprocess.run(["cc", "--version"], capture_output=True, text=True, check=True)
    output.write_text(json.dumps({
        "generated_at": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
        "probe_sha256": sha256(probe),
        "matrix_sha256": sha256(__file__),
        "compiler": compiler.stdout.splitlines()[0],
        "platform": platform.platform(),
        "cflags": CFLAGS,
        "engines": engine_provenance,
        "runs": runs,
        "all_as_expected": not failed,
    }, indent=2) + "\n")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
