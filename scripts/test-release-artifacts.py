#!/usr/bin/env python3
"""Release trust-boundary and deterministic packaging tests (#333)."""
import argparse
import contextlib
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("release_artifacts", Path(__file__).with_name("release-artifacts.py"))
release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release)


class ReleaseArtifacts(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.directory = Path(self.temporary.name)

    def downloads(self):
        archive = self.directory / "rekenraam-v0.1.0-rc.1-linux-amd64.tar.gz"
        archive.write_bytes(b"the final signed-and-packaged artifact")
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        (self.directory / "SHA256SUMS").write_text(f"{digest}  {archive.name}\n")
        return archive

    def test_authentic_manifest_checks_final_bytes_and_refuses_tampering(self):
        archive = self.downloads()
        with contextlib.redirect_stdout(io.StringIO()):
            release.verify_files(self.directory)
        archive.write_bytes(archive.read_bytes() + b"changed")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            release.verify_files(self.directory)

    def test_unsigned_extra_file_missing_file_and_symlink_are_refused(self):
        archive = self.downloads()
        extra = self.directory / "INSTALL.sh"
        extra.write_text("untrusted instructions")
        with self.assertRaisesRegex(ValueError, "every download"):
            release.verify_files(self.directory)
        extra.unlink()
        original = archive.read_bytes()
        archive.unlink()
        with self.assertRaisesRegex(ValueError, "missing/unsafe"):
            release.verify_files(self.directory)
        target = self.directory / "target"
        target.write_bytes(original)
        archive.symlink_to(target)
        with self.assertRaisesRegex(ValueError, "missing/unsafe"):
            release.verify_files(self.directory)

    def test_traversal_duplicate_empty_and_reserved_checksum_entries_are_refused(self):
        archive = self.downloads()
        manifest = self.directory / "SHA256SUMS"
        valid = manifest.read_text()
        for contents in (valid.replace(archive.name, "../outside"), valid + valid, "",
                         valid.replace(archive.name, "SHA256SUMS.sigstore.json")):
            with self.subTest(contents=contents), self.assertRaises(ValueError):
                manifest.write_text(contents)
                release.verify_files(self.directory)

    def test_candidate_label_cannot_be_final_release_or_shell_path(self):
        self.assertEqual(release.candidate_version("v0.1.0-rc.1"), "v0.1.0-rc.1")
        for version in ("v0.1.0", "latest", "../../v0.1.0-rc.1", "v0.1.0-rc.1\n",
                        "v0.1.0-rc.1;echo bad", "v01.1.0-rc.1"):
            with self.subTest(version=version), self.assertRaises(ValueError):
                release.candidate_version(version)

    def test_migration_manifest_records_every_file_and_detects_gaps(self):
        (self.directory / "0001_initial.sql").write_bytes(b"CREATE TABLE ledger(id);\n")
        (self.directory / "0002_guard.sql").write_bytes(b"CREATE TRIGGER guard;\n")
        highest, manifest = release.migration_manifest(self.directory)
        self.assertEqual(highest, 2)
        self.assertEqual(manifest.splitlines(), [
            hashlib.sha256(b"CREATE TABLE ledger(id);\n").hexdigest() + "  0001_initial.sql",
            hashlib.sha256(b"CREATE TRIGGER guard;\n").hexdigest() + "  0002_guard.sql"])
        (self.directory / "0002_guard.sql").rename(self.directory / "0003_guard.sql")
        with self.assertRaisesRegex(ValueError, "contiguous"):
            release.migration_manifest(self.directory)

    def test_archive_bytes_ignore_file_order_and_wall_clock(self):
        first, second = self.directory / "first.tgz", self.directory / "second.tgz"
        files = {"rekenraam": (b"executable", True), "LICENSE": (b"license", False)}
        release.write_archive(first, "rekenraam", files, 1700000000)
        release.write_archive(second, "rekenraam", dict(reversed(list(files.items()))), 1700000000)
        self.assertEqual(first.read_bytes(), second.read_bytes())
        with tarfile.open(first) as archive:
            self.assertEqual(archive.getnames(), ["rekenraam/LICENSE", "rekenraam/rekenraam"])
            self.assertEqual(archive.extractfile("rekenraam/rekenraam").read(), b"executable")
            self.assertEqual(archive.getmember("rekenraam/rekenraam").mode, 0o755)
            self.assertEqual(archive.getmember("rekenraam/LICENSE").mtime, 1700000000)
        with self.assertRaises(FileExistsError):
            release.write_archive(first, "rekenraam", files, 1700000000)

    def test_build_metadata_does_not_capture_the_inspected_binary_path(self):
        with patch.object(release, "run", side_effect=[
            "/home/runner/dist/rekenraam: go1.27.4\n\tbuild\t-trimpath=true\n",
            "/tmp/repeat/rekenraam: go1.27.4\n\tbuild\t-trimpath=true\n",
        ]):
            self.assertEqual(release.go_build_info(Path("one")), release.go_build_info(Path("two")))

    def test_binary_architecture_glibc_and_sqlite_contract(self):
        binary = self.directory / "rekenraam"
        binary.write_bytes(b"\x7fELF\x02\x01" + bytes(12) + bytes([62, 0]))
        def readelf(*args):
            return "GLIBC_2.34 GLIBC_2.2.5" if "--version-info" in args else "Shared library: [libc.so.6]"
        with patch.object(release, "run", side_effect=readelf):
            self.assertEqual(release.inspect_binary(binary, "amd64")["highest_glibc_symbol"], "2.34")
            with self.assertRaisesRegex(ValueError, "arm64"):
                release.inspect_binary(binary, "arm64")
        with patch.object(release, "run", return_value="GLIBC_2.40"):
            with self.assertRaisesRegex(ValueError, "no newer"):
                release.inspect_binary(binary, "amd64")
        with patch.object(release, "run", side_effect=["GLIBC_2.34", "Shared library: [libsqlite3.so.0]"]):
            with self.assertRaisesRegex(ValueError, "bundled SQLite"):
                release.inspect_binary(binary, "amd64")

    def test_assembly_refuses_mixed_source_or_missing_platform(self):
        version = "v0.1.0-rc.1"
        for arch in release.ARCHES:
            name = f"rekenraam-{version}-linux-{arch}"
            info = {"version": version, "arch": arch, "source_commit": arch,
                    "highest_migration": 2, "migrations_sha256": "a", "input_hashes": {}}
            release.write_archive(self.directory / f"{name}.tar.gz", name,
                                  {"BUILDINFO.json": (json.dumps(info).encode(), False)}, 1700000000)
        args = argparse.Namespace(version=version, input=self.directory)
        with self.assertRaisesRegex(ValueError, "disagree on source_commit"):
            release.assemble(args)
        next(self.directory.glob("*arm64*" )).unlink()
        with self.assertRaisesRegex(ValueError, "exactly"):
            release.assemble(args)

    def test_workflow_has_no_publication_or_oidc_in_build_jobs(self):
        workflow = (release.ROOT / ".github/workflows/release-candidate.yml").read_text()
        self.assertIn("workflow_dispatch:", workflow)
        self.assertNotIn("contents: write", workflow)
        self.assertNotIn("packages: write", workflow)
        self.assertNotIn("gh release", workflow)
        self.assertNotIn("git push", workflow)
        self.assertNotIn("id-token: write", workflow.split("  sign:\n")[0])
        self.assertIn("github.ref == 'refs/heads/main'", workflow)
        self.assertIn("github.repository == 'sergeyfarin/rekenraam'", workflow)
        self.assertIn("ubuntu-24.04-arm", workflow)
        self.assertIn("needs: build", workflow)


if __name__ == "__main__":
    unittest.main()
