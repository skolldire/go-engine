#!/usr/bin/env bash
# Rewrites every intra-repository require to the given version.
#
# The modules in this repository are released together, so a submodule must
# require the exact core version tagged in the same release. Doing it by hand is
# how a submodule ends up requiring a core that was never published: invisible
# locally, because go.work and the replace directives satisfy it, and broken
# only for the consumer.
#
# Usage: scripts/sync-module-versions.sh v0.30.0
set -euo pipefail

VERSION="${1:-}"
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "usage: $0 vX.Y.Z" >&2
    exit 1
fi

cd "$(dirname "$0")/.."

# e2e is never published, so its requires stay on replace directives.
for dir in $(find . -name go.mod -not -path './.git/*' -not -path './example/*' -exec dirname {} \; | sort); do
    gomod="$dir/go.mod"
    if grep -qE '^[[:space:]]+github\.com/skolldire/go-engine(/[a-z/]+)? v' "$gomod"; then
        perl -pi -e "s{^(\\s+github\\.com/skolldire/go-engine(?:/[a-z/]+)?) v\\S+}{\$1 $VERSION}" "$gomod"
        echo "  updated $gomod"
    fi
done

echo "==> All intra-repository requires set to $VERSION"
