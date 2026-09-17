"""Release regressions use a fake compiler, never a real cross-compilation."""
import argparse
import hashlib
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import release


class ReleaseTests(unittest.TestCase):
    def test_failed_compile_keeps_previous_release(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
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
            self.assertEqual(len(lines), 5)
            for line in lines:
                digest, name = line.split()
                self.assertEqual(digest, hashlib.sha256((output / name).read_bytes()).hexdigest())

    def test_version_cannot_add_linker_flags_or_commands(self):
        for value in ("", "v1 -X main.version=other", "v1\nextra", "v1';echo hi", "v1$(id)", "x" * 129):
            with self.subTest(value=value), self.assertRaises(argparse.ArgumentTypeError):
                release.release_version(value)


if __name__ == "__main__":
    unittest.main()
