#!/usr/bin/env bash
# Run as root inside an Arch base-devel container; /dist is the CI artifact mount.
set -euo pipefail
pacman -Syu --noconfirm ca-certificates
useradd --create-home builder
cp -R /dist/packaging/aur /home/builder/package
cp /dist/release/atlo-*-linux-amd64.tar.gz /home/builder/package/
chown -R builder:builder /home/builder/package
cd /home/builder/package
# Validate the published metadata before replacing remote sources for offline building.
runuser -u builder -- makepkg --printsrcinfo > /tmp/actual.SRCINFO
diff -u .SRCINFO /tmp/actual.SRCINFO
# makepkg uses an already-downloaded file when its source filename is present.
runuser -u builder -- makepkg --cleanbuild --noconfirm
pacman -U --noconfirm ./atlo-bin-*.pkg.tar.zst
atlo version
atlo schema jira issue get
[[ -f /usr/share/licenses/atlo-bin/LICENSE ]]
[[ -f /usr/share/licenses/atlo-bin/THIRD_PARTY_NOTICES ]]
mkdir -p /dist/arch
cp ./atlo-bin-*.pkg.tar.zst /dist/arch/
