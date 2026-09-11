#!/bin/sh
# Builds the release archives and their checksums into dist/.
#
# Only the Go toolchain is used, so the build needs nothing that is not
# already required to work on the project. Nothing is published: the files
# are left in dist/ for a person to inspect and upload.
#
# Usage: scripts/build-release.sh [version]
set -eu

cd "$(dirname "$0")/.." || exit 2

version=${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
out=dist
rm -rf "$out"
mkdir -p "$out"

# CGO is off so each archive holds one static executable. -trimpath keeps
# build machine paths out of the binary.
export CGO_ENABLED=0

targets="darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/amd64"

for target in $targets; do
	os=${target%/*}
	arch=${target#*/}
	name="zeroturn_${version}_${os}_${arch}"
	dir="$out/$name"
	mkdir -p "$dir"

	bin="zeroturn"
	if [ "$os" = "windows" ]; then
		bin="zeroturn.exe"
	fi

	GOOS="$os" GOARCH="$arch" go build \
		-trimpath \
		-ldflags "-s -w -X main.Version=$version" \
		-o "$dir/$bin" ./cmd/zeroturn

	cp README.md LICENSE COPYING CHANGELOG.md "$dir/"

	if [ "$os" = "windows" ]; then
		(cd "$out" && zip -q -r "$name.zip" "$name")
	else
		tar -czf "$out/$name.tar.gz" -C "$out" "$name"
	fi
	rm -rf "$dir"
	printf '%-44s %s\n' "$name" "$(du -h "$out"/"$name".* | cut -f1)"
done

(
	cd "$out"
	: >SHA256SUMS
	for archive in *.tar.gz *.zip; do
		if command -v sha256sum >/dev/null 2>&1; then
			sha256sum "$archive" >>SHA256SUMS
		else
			shasum -a 256 "$archive" >>SHA256SUMS
		fi
	done
)

echo
echo "Archives and SHA256SUMS are in $out/. Nothing has been published."
echo "Publishing a release is a separate, deliberate step: see docs/release.md."
