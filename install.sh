#!/usr/bin/env bash
# Install a checksum-verified atlo release without a compiler or package manager.
set -euo pipefail
umask 022

usage() {
  cat <<'USAGE'
Usage: bash install.sh [--version vX.Y.Z] [--prefix DIRECTORY]

Install atlo for macOS or Linux (Apple Silicon/ARM64 or x86-64).
Defaults: latest release, installed under $HOME/.local.
The executable goes in PREFIX/bin and license notices in PREFIX/share/licenses/atlo.
No shell configuration is changed and sudo is never invoked automatically.
USAGE
}
fail() { printf 'atlo: %s\n' "$*" >&2; exit 1; }
version=''
prefix="${HOME:?HOME must be set}/.local"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version|--prefix)
      [[ $# -ge 2 && -n "$2" && "$2" != --* ]] || fail "$1 requires a value"
      if [[ "$1" == --version ]]; then version="$2"; else prefix="$2"; fi
      shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) fail "Unknown argument: $1 (use --help)" ;;
  esac
done
[[ "$prefix" == /* ]] || fail '--prefix must be an absolute path'
[[ "$prefix" != *$'\n'* && "$prefix" != *$'\r'* ]] || fail 'Invalid prefix'
if [[ -n "$version" && ! "$version" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  fail '--version must be a stable release tag such as v0.1.0'
fi

case "$(uname -s)" in
  Darwin) system=darwin ;;
  Linux) system=linux ;;
  *) fail 'Supported operating systems: macOS and Linux' ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) fail 'Supported CPU architectures: x86-64 and ARM64' ;;
esac
for tool in curl tar awk mktemp install mv; do
  command -v "$tool" >/dev/null 2>&1 || fail "Required command not found: $tool"
done
if command -v sha256sum >/dev/null 2>&1; then
  checksum() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  checksum() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  fail 'Install sha256sum or shasum before continuing'
fi
fetch() {
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --tlsv1.2 --connect-timeout 15 --max-time 180 --retry 2 "$@"
}
repository='https://github.com/metryon/atlo'
if [[ -z "$version" ]]; then
  latest=$(fetch --output /dev/null --write-out '%{url_effective}' "$repository/releases/latest")
  [[ "$latest" == "$repository/releases/tag/"* ]] || fail 'Could not resolve the latest release'
  version=${latest##*/}
fi
[[ "$version" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || fail 'Invalid release version'
archive="atlo-${version}-${system}-${arch}.tar.gz"
temporary=$(mktemp -d "${TMPDIR:-/tmp}/atlo-install.XXXXXXXX")
staged_binary=''
cleanup() {
  rm -rf -- "$temporary"
  if [[ -n "$staged_binary" ]]; then rm -f -- "$staged_binary"; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
chmod 700 "$temporary"
printf 'Downloading atlo %s for %s/%s...\n' "$version" "$system" "$arch"
fetch --output "$temporary/$archive" "$repository/releases/download/$version/$archive"
fetch --output "$temporary/SHA256SUMS" "$repository/releases/download/$version/SHA256SUMS"
expected=$(awk -v name="$archive" '$2 == name { count++; digest=$1 } END { if (count != 1) exit 1; print digest }' "$temporary/SHA256SUMS") \
  || fail 'Release checksum is missing or duplicated'
[[ "$expected" =~ ^[a-f0-9]{64}$ ]] || fail 'Release checksum is invalid'
[[ "$(checksum "$temporary/$archive")" == "$expected" ]] || fail 'Checksum mismatch; installation stopped'

# Reject paths, links, and extra files before extracting into the private directory.
listing=$(tar -tzf "$temporary/$archive") || fail 'Invalid release archive'
count=0
while IFS= read -r entry; do
  case "$entry" in atlo|LICENSE|THIRD_PARTY_NOTICES|README.md) ;; *) fail 'Unexpected archive member' ;; esac
  count=$((count + 1))
done <<< "$listing"
[[ "$count" == 4 ]] || fail 'Unexpected archive contents'
tar -tvzf "$temporary/$archive" | awk 'substr($0,1,1) != "-" {bad=1} END {exit bad}' \
  || fail 'Archive must contain only regular files'
tar -xzf "$temporary/$archive" -C "$temporary"
for entry in atlo LICENSE THIRD_PARTY_NOTICES README.md; do
  [[ -f "$temporary/$entry" && ! -L "$temporary/$entry" ]] || fail "Missing regular file: $entry"
done
[[ ! -d "$prefix/bin/atlo" ]] || fail 'The destination executable is a directory'
mkdir -p "$prefix/bin" "$prefix/share/licenses/atlo" "$prefix/share/doc/atlo"
staged_binary=$(mktemp "$prefix/bin/.atlo.XXXXXXXX")
install -m 755 "$temporary/atlo" "$staged_binary"
install -m 644 "$temporary/LICENSE" "$prefix/share/licenses/atlo/LICENSE"
install -m 644 "$temporary/THIRD_PARTY_NOTICES" "$prefix/share/licenses/atlo/THIRD_PARTY_NOTICES"
install -m 644 "$temporary/README.md" "$prefix/share/doc/atlo/README.md"
mv -f "$staged_binary" "$prefix/bin/atlo"
staged_binary=''
printf 'Installed atlo %s to %s/bin/atlo\n' "$version" "$prefix"
case ":${PATH:-}:" in
  *":$prefix/bin:"*) ;;
  *) printf 'Add %s/bin to your shell PATH to run atlo by name.\n' "$prefix" ;;
esac
printf 'Check which copy your shell uses with: command -v atlo\n'
