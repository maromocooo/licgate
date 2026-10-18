#!/usr/bin/env bash
# Build release archives and Debian/Ubuntu packages.
#
#   scripts/package.sh [VERSION]
#
# Produces in dist/:
#   licgate_<ver>_linux_amd64.tar.gz   licgate_<ver>_amd64.deb
#   licgate_<ver>_linux_arm64.tar.gz   licgate_<ver>_arm64.deb
#   licgate_<ver>_darwin_arm64.tar.gz  licgate_<ver>_darwin_amd64.tar.gz
#   checksums.txt
#
# Binaries are static (CGO_ENABLED=0), so they run on any Ubuntu/Debian
# release without extra libraries. Needs Go and dpkg-deb.
set -euo pipefail

cd "$(dirname "$0")/.."
VERSION="${1:-$(git describe --tags --always 2>/dev/null || echo 0.0.0-dev)}"
VERSION="${VERSION#v}"
# Debian versions must start with a digit.
DEB_VERSION="$VERSION"
[[ "$DEB_VERSION" =~ ^[0-9] ]] || DEB_VERSION="0.0.0+${DEB_VERSION}"
MAINTAINER="${LICGATE_MAINTAINER:-maromocooo <maromocooo@users.noreply.github.com>}"
HOMEPAGE="${LICGATE_HOMEPAGE:-https://github.com/maromocooo/licgate}"

rm -rf dist
mkdir -p dist

build() { # os arch outdir
	CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -trimpath \
		-ldflags "-s -w -X main.version=${VERSION}" -o "$3/licgate" ./cmd/licgate
}

docs=(README.md README.ja.md)
[[ -f LICENSE ]] && docs+=(LICENSE)

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
	os="${target%/*}"
	arch="${target#*/}"
	name="licgate_${VERSION}_${os}_${arch}"
	stage="dist/stage/${name}"
	mkdir -p "$stage"
	build "$os" "$arch" "$stage"
	cp "${docs[@]}" "$stage/"
	tar -C dist/stage -czf "dist/${name}.tar.gz" "$name"

	if [[ "$os" == linux ]]; then
		pkg="dist/stage/deb_${arch}"
		mkdir -p "$pkg/DEBIAN" "$pkg/usr/bin" "$pkg/usr/share/doc/licgate"
		install -m 0755 "$stage/licgate" "$pkg/usr/bin/licgate"
		install -m 0644 "${docs[@]}" "$pkg/usr/share/doc/licgate/"
		cat >"$pkg/DEBIAN/control" <<EOF
Package: licgate
Version: ${DEB_VERSION}
Section: devel
Priority: optional
Architecture: ${arch}
Maintainer: ${MAINTAINER}
Homepage: ${HOMEPAGE}
Recommends: git
Suggests: golang-go, cargo, nodejs, python3
Description: check dependency licenses before you publish
 licgate scans a project's dependencies (npm, Python, Go, Rust, PHP, or any
 CycloneDX/SPDX SBOM) and tells you whether they are compatible with the
 license you intend to publish under, with an interactive TUI, reports,
 third-party notices and CI exit codes.
EOF
		dpkg-deb --root-owner-group --build "$pkg" "dist/licgate_${DEB_VERSION}_${arch}.deb" >/dev/null
	fi
done

rm -rf dist/stage
(cd dist && sha256sum ./*.tar.gz ./*.deb | sed 's# \./# #' >checksums.txt)
ls -1 dist
