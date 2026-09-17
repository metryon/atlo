"""Cross-compile a complete release before replacing the previous artifacts."""
import argparse
import hashlib
import gzip
import os
from pathlib import Path
import re
import subprocess
import tempfile
import tarfile
import zipfile

TARGETS = (("darwin", "arm64"), ("darwin", "amd64"),
           ("linux", "arm64"), ("linux", "amd64"), ("windows", "amd64"))
DOCUMENTS = ("LICENSE", "THIRD_PARTY_NOTICES", "README.md")


def archive(staging, root, binary, system, arch, version):
    """Stable archive metadata; extract directly to a binary and its notices."""
    extension = "zip" if system == "windows" else "tar.gz"
    name = f"atlo-{version}-{system}-{arch}.{extension}"
    members = [(binary, "atlo.exe" if system == "windows" else "atlo", 0o755)]
    members.extend((root / name, name, 0o644) for name in DOCUMENTS)
    if system == "windows":
        with zipfile.ZipFile(staging / name, "w", compression=zipfile.ZIP_DEFLATED) as bundle:
            for source, target, mode in members:
                info = zipfile.ZipInfo(target, (1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.external_attr = (0o100000 | mode) << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                bundle.writestr(info, source.read_bytes())
    else:
        with (staging / name).open("wb") as raw:
            with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=0) as compressed:
                with tarfile.open(fileobj=compressed, mode="w") as bundle:
                    for source, target, mode in members:
                        info = tarfile.TarInfo(target)
                        info.size = source.stat().st_size
                        info.mode = mode
                        with source.open("rb") as stream:
                            bundle.addfile(info, stream)
    return name


def release_version(value):
    """A version is one linker value, never a second linker flag."""
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._+-]{0,127}", value):
        raise argparse.ArgumentTypeError("version must be 1–128 letters, digits, dots, underscores, pluses, or hyphens")
    return value


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def build_release(root, go, version):
    version = release_version(version)
    output = root / "bin" / "release"
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".atlo-release-", dir=output.parent) as temporary:
        staging = Path(temporary)
        artifacts = []
        checksums = []
        for system, arch in TARGETS:
            name = f"atlo-{system}-{arch}" + (".exe" if system == "windows" else "")
            target = staging / name
            subprocess.run([
                go, "build", "-mod=readonly", "-trimpath", "-ldflags",
                f"-s -w -X main.version={version}", "-o", str(target), "./cmd/atlo"
            ], cwd=root, env={**os.environ, "CGO_ENABLED": "0", "GOOS": system,
                             "GOARCH": arch, "GOAMD64": "v1", "GOARM64": "v8.0"}, check=True)
            checksums.append(f"{sha256(target)}  {name}\n")
            artifacts.append(name)
            bundle = archive(staging, root, target, system, arch, version)
            checksums.append(f"{sha256(staging / bundle)}  {bundle}\n")
            artifacts.append(bundle)
        for name in DOCUMENTS:
            (staging / name).write_bytes((root / name).read_bytes())
            checksums.append(f"{sha256(staging / name)}  {name}\n")
            artifacts.append(name)
        (staging / "SHA256SUMS").write_text("".join(checksums), encoding="utf-8")
        output.mkdir(exist_ok=True)
        # Compilation failures never touch the old release. Commit checksums last;
        # interrupted promotion is detectable by verification, not an atomic bundle.
        for name in [*artifacts, "SHA256SUMS"]:
            os.replace(staging / name, output / name)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--go", default="go")
    parser.add_argument("--version", type=release_version, default="dev")
    args = parser.parse_args()
    build_release(Path(__file__).resolve().parent.parent, args.go, args.version)


if __name__ == "__main__":
    main()
