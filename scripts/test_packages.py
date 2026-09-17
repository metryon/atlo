"""Checks package metadata against the actual artifacts it installs."""
import argparse
from pathlib import Path
import tempfile
import unittest

import packages
from release import sha256


class PackageTests(unittest.TestCase):
    def test_metadata_contains_matching_urls_versions_and_hashes(self):
        root = Path(__file__).resolve().parent.parent
        with tempfile.TemporaryDirectory() as directory:
            release = Path(directory)
            sums = []
            for system in ("darwin", "linux"):
                for arch in ("arm64", "amd64"):
                    p = release / f"atlo-v0.1.0-{system}-{arch}.tar.gz"
                    p.write_bytes(f"{system}/{arch}".encode())
                    sums.append(f"{sha256(p)}  {p.name}\n")
            (release / "SHA256SUMS").write_text("".join(sums))
            output = release / "packaging"
            packages.generate(root, release, output, "v0.1.0")
            formula = (output / "homebrew/atlo.rb").read_text()
            pkgbuild = (output / "aur/PKGBUILD").read_text()
            srcinfo = (output / "aur/.SRCINFO").read_text()
            for line in sums:
                digest, name = line.split()
                if "darwin" in name:
                    self.assertIn(digest, formula)
                    self.assertIn(name, formula)
                else:
                    for text in (pkgbuild, srcinfo):
                        self.assertIn(digest, text)
                        self.assertIn(name, text)
            self.assertIn('version "0.1.0"', formula)
            self.assertIn('pkgver=0.1.0', pkgbuild)
            self.assertIn('pkgver = 0.1.0', srcinfo)
            self.assertNotIn('SKIP', pkgbuild)
            (release / "atlo-v0.1.0-linux-amd64.tar.gz").write_bytes(b"tampered")
            with self.assertRaises(ValueError):
                packages.generate(root, release, output, "v0.1.0")

    def test_only_stable_version_tags_can_enter_package_templates(self):
        for value in ("dev", "1.0.0", "v1.0.0-rc.1", "v01.0.0", "v1.0.0;id", "v1.0.0\n"):
            with self.subTest(value=value), self.assertRaises(argparse.ArgumentTypeError):
                packages.package_version(value)


if __name__ == "__main__":
    unittest.main()
