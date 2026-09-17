"""Render package definitions against verified, versioned release artifacts."""
import argparse
from pathlib import Path
import re

from release import sha256


def package_version(value):
    if not re.fullmatch(r"v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)", value):
        raise argparse.ArgumentTypeError("package releases require a tag such as v0.1.0")
    return value


def generate(root, release, output, tag):
    tag = package_version(tag)
    sums = {}
    for line in (release / "SHA256SUMS").read_text().splitlines():
        digest, name = line.split()
        if name in sums or Path(name).name != name or not re.fullmatch(r"[a-f0-9]{64}", digest):
            raise ValueError("invalid checksum manifest")
        sums[name] = digest
    values = {"TAG": tag, "VERSION": tag[1:]}
    for system in ("darwin", "linux"):
        for arch in ("arm64", "amd64"):
            name = f"atlo-{tag}-{system}-{arch}.tar.gz"
            digest = sums.get(name)
            if digest is None or sha256(release / name) != digest:
                raise ValueError(f"missing or mismatched release archive: {name}")
            values[f"{system}_{arch}_SHA256".upper()] = digest
    rendered = {}
    for source, target in (("homebrew/atlo.rb.in", "homebrew/atlo.rb"),
                           ("aur/PKGBUILD.in", "aur/PKGBUILD"),
                           ("aur/SRCINFO.in", "aur/.SRCINFO")):
        text = (root / "packaging" / source).read_text()
        for key, value in values.items():
            text = text.replace(f"@{key}@", value)
        if re.search(r"@[A-Z0-9_]+@", text):
            raise ValueError(f"unresolved placeholder in {source}")
        rendered[target] = text
    for target, text in rendered.items():
        path = output / target
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)


def main():
    root = Path(__file__).resolve().parent.parent
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True, type=package_version)
    parser.add_argument("--release-dir", type=Path, default=root / "bin/release")
    parser.add_argument("--output", type=Path, default=root / "bin/packaging")
    args = parser.parse_args()
    generate(root, args.release_dir, args.output, args.version)


if __name__ == "__main__":
    main()
