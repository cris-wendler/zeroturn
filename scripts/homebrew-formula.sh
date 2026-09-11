#!/bin/sh
# Fills packaging/homebrew/zeroturn.rb.template with a version and the
# checksums from dist/SHA256SUMS, and writes the formula to standard output.
#
# The formula belongs in a separate tap repository. Nothing is published
# by this script.
#
# Usage: scripts/build-release.sh 0.1.0 && scripts/homebrew-formula.sh 0.1.0
set -eu

cd "$(dirname "$0")/.." || exit 2
version=${1:?usage: scripts/homebrew-formula.sh <version>}
sums=dist/SHA256SUMS
[ -f "$sums" ] || { echo "$sums is missing, run scripts/build-release.sh $version first" >&2; exit 2; }

sum_for() {
	value=$(grep "zeroturn_${version}_$1.tar.gz" "$sums" | cut -d' ' -f1)
	[ -n "$value" ] || { echo "no checksum for $1 in $sums" >&2; exit 2; }
	echo "$value"
}

sed \
	-e "s/@VERSION@/$version/g" \
	-e "s/@SHA_DARWIN_ARM64@/$(sum_for darwin_arm64)/" \
	-e "s/@SHA_DARWIN_AMD64@/$(sum_for darwin_amd64)/" \
	-e "s/@SHA_LINUX_ARM64@/$(sum_for linux_arm64)/" \
	-e "s/@SHA_LINUX_AMD64@/$(sum_for linux_amd64)/" \
	packaging/homebrew/zeroturn.rb.template
