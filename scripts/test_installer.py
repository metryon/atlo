"""Exercise the actual Bash installer with local release assets and a fake network."""
import hashlib
import io
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest


INSTALLER = Path(__file__).resolve().parent.parent / "install.sh"


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.assets = self.root / "assets"
        self.assets.mkdir()
        self.commands = self.root / "commands"
        self.commands.mkdir()
        self.prefix = self.root / "install with spaces"
        self.env = {**os.environ, "PATH": str(self.commands) + os.pathsep + os.environ["PATH"],
                    "FIXTURES": str(self.assets), "TEST_OS": "Linux", "TEST_ARCH": "x86_64"}
        curl = self.commands / "curl"
        curl.write_text('#!' + sys.executable + '\n' + '''
import os, pathlib, shutil, sys
args = sys.argv[1:]
assert args[args.index('--proto')+1] == '=https'
assert args[args.index('--proto-redir')+1] == '=https'
assert args[-1].startswith('https://github.com/metryon/atlo/releases/')
if args[-1].endswith('/latest'):
    print('https://github.com/metryon/atlo/releases/tag/v0.1.0', end='')
else:
    shutil.copyfile(pathlib.Path(os.environ['FIXTURES']) / args[-1].rsplit('/',1)[1], args[args.index('--output')+1])
''')
        curl.chmod(0o755)
        uname = self.commands / "uname"
        uname.write_text('#!/bin/sh\ncase "$1" in -s) printf "%s\\n" "$TEST_OS";; -m) printf "%s\\n" "$TEST_ARCH";; esac\n')
        uname.chmod(0o755)

    def artifact(self, system="linux", arch="amd64", malicious=False):
        name = f"atlo-v0.1.0-{system}-{arch}.tar.gz"
        archive = self.assets / name
        with tarfile.open(archive, "w:gz") as bundle:
            for member in ("atlo", "LICENSE", "THIRD_PARTY_NOTICES", "README.md"):
                payload = b"#!/bin/sh\necho atlo\n" if member == "atlo" else b"license/documentation\n"
                info = tarfile.TarInfo("../escape" if malicious and member == "README.md" else member)
                info.size = len(payload)
                info.mode = 0o755 if member == "atlo" else 0o644
                bundle.addfile(info, io.BytesIO(payload))
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        (self.assets / "SHA256SUMS").write_text(f"{digest}  {name}\n")
        return archive

    def run_installer(self, *args):
        return subprocess.run(['/bin/bash', str(INSTALLER), '--prefix', str(self.prefix), *args],
                              env=self.env, text=True, capture_output=True)

    def test_native_targets_install_binary_and_licenses(self):
        for system, machine, asset_system, arch in [('Linux','x86_64','linux','amd64'),
              ('Linux','aarch64','linux','arm64'),('Darwin','arm64','darwin','arm64'),
              ('Darwin','x86_64','darwin','amd64')]:
            with self.subTest(system=system, machine=machine):
                self.env.update(TEST_OS=system, TEST_ARCH=machine)
                self.artifact(asset_system, arch)
                # No --version also exercises resolving the latest release URL.
                result = self.run_installer()
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertTrue(os.access(self.prefix / 'bin/atlo', os.X_OK))
                self.assertTrue((self.prefix / 'share/licenses/atlo/LICENSE').is_file())
                self.assertTrue((self.prefix / 'share/licenses/atlo/THIRD_PARTY_NOTICES').is_file())
                self.assertFalse(list((self.prefix / 'bin').glob('.atlo.*')))

    def test_mismatch_preserves_existing_binary(self):
        archive = self.artifact()
        archive.write_bytes(b'tampered')
        (self.prefix / 'bin').mkdir(parents=True)
        (self.prefix / 'bin/atlo').write_bytes(b'previous binary')
        result = self.run_installer('--version', 'v0.1.0')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Checksum mismatch', result.stderr)
        self.assertEqual((self.prefix / 'bin/atlo').read_bytes(), b'previous binary')

    def test_archive_paths_and_bad_options_are_rejected(self):
        self.artifact(malicious=True)
        result = self.run_installer('--version', 'v0.1.0')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Unexpected archive member', result.stderr)
        self.assertFalse((self.prefix / 'bin/atlo').exists())
        for args in [('--version','v0.1.0;id'), ('--prefix','relative'), ('--unknown',), ('--version',)]:
            with self.subTest(args=args):
                self.assertNotEqual(self.run_installer(*args).returncode, 0)
        self.env['TEST_ARCH'] = 'riscv64'
        self.assertNotEqual(self.run_installer('--version','v0.1.0').returncode, 0)

    def test_duplicate_checksum_is_rejected(self):
        self.artifact()
        sums = self.assets / 'SHA256SUMS'
        sums.write_text(sums.read_text() * 2)
        result = self.run_installer('--version','v0.1.0')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('missing or duplicated', result.stderr)


if __name__ == '__main__':
    unittest.main()
