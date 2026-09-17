"""Release regressions use a fake compiler, never a real cross-compilation."""
import argparse
import hashlib
from pathlib import Path
import subprocess
import tempfile
import unittest
import tarfile
import zipfile
from unittest.mock import patch

import release


class ReleaseTests(unittest.TestCase):
    def prepare_documents(self, root):
        for name in release.DOCUMENTS:
            (root / name).write_text(f"Contents of {name}\n")

    def test_failed_compile_keeps_previous_release(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.prepare_documents(root)
            output = root / "bin" / "release"
            output.mkdir(parents=True)
            for name in ("atlo-darwin-arm64", "SHA256SUMS"):
                (output / name).write_bytes(b"previous release")
            calls = 0

            def compiler(command, **kwargs):
                nonlocal calls
                calls += 1
                if calls == 2:
                    raise subprocess.CalledProcessError(1, command)
                Path(command[command.index("-o") + 1]).write_bytes(b"new binary")

            with patch.object(release.subprocess, "run", side_effect=compiler):
                with self.assertRaises(subprocess.CalledProcessError):
                    release.build_release(root, "go", "v1.2.3")
            self.assertEqual({p.name: p.read_bytes() for p in output.iterdir()},
                             {"atlo-darwin-arm64": b"previous release", "SHA256SUMS": b"previous release"})
            self.assertEqual(list(output.parent.glob(".atlo-release-*")), [])

    def test_success_has_matching_checksums_and_portable_targets(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.prepare_documents(root)

            def compiler(command, *, cwd, env, check):
                self.assertEqual(cwd, root)
                self.assertTrue(check)
                self.assertIn("-mod=readonly", command)
                self.assertEqual(env["CGO_ENABLED"], "0")
                self.assertEqual(env["GOAMD64"], "v1")
                self.assertEqual(env["GOARM64"], "v8.0")
                Path(command[command.index("-o") + 1]).write_bytes(
                    f'{env["GOOS"]}/{env["GOARCH"]}'.encode())

            with patch.object(release.subprocess, "run", side_effect=compiler):
                release.build_release(root, "go", "v1.2.3-rc.1+build")
            output = root / "bin" / "release"
            lines = (output / "SHA256SUMS").read_text().splitlines()
            self.assertEqual(len(lines), 13)
            for line in lines:
                digest, name = line.split()
                self.assertEqual(digest, hashlib.sha256((output / name).read_bytes()).hexdigest())
            for system, arch in release.TARGETS:
                name = f"atlo-v1.2.3-rc.1+build-{system}-{arch}"
                expected = set(release.DOCUMENTS) | {"atlo.exe" if system == "windows" else "atlo"}
                if system == "windows":
                    with zipfile.ZipFile(output / (name + ".zip")) as bundle:
                        self.assertEqual(set(bundle.namelist()), expected)
                        self.assertEqual(bundle.read("atlo.exe"), b"windows/amd64")
                else:
                    with tarfile.open(output / (name + ".tar.gz")) as bundle:
                        self.assertEqual(set(bundle.getnames()), expected)
                        self.assertEqual(bundle.getmember("atlo").mode, 0o755)
                        self.assertEqual(bundle.extractfile("atlo").read(), f"{system}/{arch}".encode())

    def test_archives_are_reproducible_and_require_notices(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.prepare_documents(root)
            binary = root / "binary"
            binary.write_bytes(b"binary")
            name = release.archive(root, root, binary, "linux", "amd64", "v1.0.0")
            before = (root / name).read_bytes()
            release.archive(root, root, binary, "linux", "amd64", "v1.0.0")
            self.assertEqual((root / name).read_bytes(), before)
            (root / "LICENSE").unlink()
            with self.assertRaises(FileNotFoundError):
                release.archive(root, root, binary, "linux", "amd64", "v1.0.0")

    def test_version_cannot_add_linker_flags_or_commands(self):
        for value in ("", "v1 -X main.version=other", "v1\nextra", "v1';echo hi", "v1$(id)", "x" * 129):
            with self.subTest(value=value), self.assertRaises(argparse.ArgumentTypeError):
                release.release_version(value)


if __name__ == "__main__":
    unittest.main()
