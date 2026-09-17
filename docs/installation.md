# Installation and distribution

Release binaries require no Go installation. macOS and Linux archives cover
ARM64 and x86-64; Windows releases include an x86-64 ZIP archive. Each archive
contains the executable, MIT license, third-party notices, and README.

## Homebrew on macOS

```sh
brew install metryon/tap/atlo
brew upgrade metryon/tap/atlo
brew uninstall atlo
```

The [Metryon tap](https://github.com/metryon/homebrew-tap) installs the appropriate
prebuilt macOS archive and verifies its pinned checksum. On Homebrew versions
that request trust for non-official taps, trust this formula specifically with
`brew trust --formula metryon/tap/atlo`.

## Bash installer on macOS and Linux

```sh
curl -fL https://github.com/metryon/atlo/releases/latest/download/install.sh -o install-atlo.sh
# Review install-atlo.sh before executing it.
bash install-atlo.sh
```

The default prefix is `$HOME/.local`. To choose a version and prefix:

```sh
bash install-atlo.sh --version v0.1.0 --prefix "$HOME/.local"
```

The script uses Bash, curl, tar, and either `sha256sum` or `shasum`. It detects
the OS and CPU, downloads via HTTPS, verifies the selected archive against the
release checksum manifest, and replaces the executable only after successful
download, validation, and staging. It installs licenses under
`PREFIX/share/licenses/atlo` and documentation under `PREFIX/share/doc/atlo`.
Checksums detect corruption or mismatched assets; they are not signed provenance.

For Bash/zsh, add `export PATH="$HOME/.local/bin:$PATH"` to the appropriate shell
configuration. For fish, use `fish_add_path "$HOME/.local/bin"`. Run
`command -v atlo` to check which copy is selected, especially if you previously
added this checkout's `bin` directory to PATH.

Re-run the installer to upgrade. It does not upgrade automatically. To remove a
default-prefix installation:

```sh
rm "$HOME/.local/bin/atlo"
rm -r "$HOME/.local/share/licenses/atlo" "$HOME/.local/share/doc/atlo"
```

Linux distribution detection is unnecessary for the current static Go binaries.
The installer's OS/architecture selection can be extended if future platforms
need different artifacts. Unsupported systems fail explicitly.

## Arch Linux package without AUR

The release workflow builds and installs the package in an Arch Linux container.
For the initial x86-64 release:

```sh
curl -fLO https://github.com/metryon/atlo/releases/download/v0.1.0/atlo-bin-0.1.0-1-x86_64.pkg.tar.zst
curl -fLO https://github.com/metryon/atlo/releases/download/v0.1.0/SHA256SUMS
awk '$2 == "atlo-bin-0.1.0-1-x86_64.pkg.tar.zst"' SHA256SUMS | sha256sum -c -
sudo pacman -U ./atlo-bin-0.1.0-1-x86_64.pkg.tar.zst
```

Proceed with installation only if checksum verification succeeds. Download and
verify the newer package and repeat `pacman -U` to upgrade; these downloads are
not an official pacman repository and `pacman -Syu` does not fetch their updates.
Remove it with `sudo pacman -R atlo-bin`.

### AUR status and submission

AUR publication is pending an account with an authorized SSH key. Package names
have been checked, but no AUR package is claimed or published by this repository
yet. An existing AUR account holder can publish the prepared `atlo-bin` recipe.

Each release includes `atlo-packaging-vX.Y.Z.tar.gz` with `aur/PKGBUILD`,
`aur/.SRCINFO`, and `homebrew/atlo.rb`. The AUR recipe downloads the same Linux
archive with a pinned checksum. It supports x86-64 and ARM64; the release CI
currently builds the native Arch package for x86-64 only.

After reviewing the generated recipe on Arch, run `makepkg -si` as a normal user
to build and install it. For submission, clone
`ssh://aur@aur.archlinux.org/atlo-bin.git`, copy `PKGBUILD` and `.SRCINFO` into that
checkout, verify `.SRCINFO` with `makepkg --printsrcinfo`, and push a Conventional
Commit. Do not upload binaries or private keys to the AUR Git repository.

## Maintainer release flow

Run the Release workflow manually with a `vX.Y.Z` version to build and test without
publishing. A pushed `vX.Y.Z` Git tag builds the tagged code, tests native binaries
on macOS and Windows, builds/installs the Arch package, and then publishes a
GitHub Release. Failed checks prevent publication. Assets are uploaded to a draft
first; only the completed release becomes the latest public release.

The packaging definitions are generated from the actual archives and verified
checksums. Download the packaging archive for that release, copy
`homebrew/atlo.rb` to `Formula/atlo.rb` in the tap, run `brew test`, and commit/push
with Conventional Commits. Updating the separate tap and AUR repositories is an
explicit maintainer step; the release workflow does not store their credentials.
Never replace an already-published release's binaries without issuing a new version.

Local preparation:

```sh
make release VERSION=v0.1.0
python3 scripts/packages.py --version v0.1.0
```

Generated packaging files are under `bin/packaging` and release files under
`bin/release`; both are ignored by Git. Templates remain in `packaging/`.
