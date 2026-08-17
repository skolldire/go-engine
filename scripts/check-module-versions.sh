#!/usr/bin/env bash
# Fails when an intra-repository require names a version that cannot be resolved
# by a consumer.
#
# This check exists because nothing else can catch the problem. Inside the repo,
# go.work and the replace directives satisfy every intra-repository import, so a
# submodule requiring `v0.0.0-00010101000000-000000000000` — the placeholder
# `go mod tidy` writes when only a replace is available — builds and tests
# perfectly. A consumer's `go get` then fails with
# "invalid version: unknown revision 000000000000", because replace directives
# in a dependency's go.mod are ignored.
#
# An external smoke test does not catch it either: the moment the consumer
# replaces the core as well, the bogus require is satisfied again.
set -euo pipefail

cd "$(dirname "$0")/.."

PLACEHOLDER='v0.0.0-00010101000000-000000000000'
FAILED=0
declare -a VERSIONS=()

for gomod in $(find . -name go.mod -not -path './.git/*' | sort); do
    while read -r mod version; do
        [[ -z "${version:-}" ]] && continue

        if [[ "$version" == "$PLACEHOLDER" ]]; then
            echo "VIOLATION: $gomod requires $mod at the unresolvable placeholder $version"
            echo "  a consumer cannot install this module; run scripts/sync-module-versions.sh vX.Y.Z"
            FAILED=1
            continue
        fi

        if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]]; then
            echo "VIOLATION: $gomod requires $mod at a non-release version $version"
            FAILED=1
            continue
        fi

        VERSIONS+=("$version")
    done < <(grep -E '^[[:space:]]+github\.com/skolldire/go-engine(/[a-z/]+)? v' "$gomod" || true)
done

# Every module in this repository is released together, so a single release must
# not mix versions: that is how a submodule ends up pinned to a stale core.
UNIQUE=$(printf '%s\n' "${VERSIONS[@]:-}" | sort -u | grep -c . || true)
if [[ "$UNIQUE" -gt 1 ]]; then
    echo "VIOLATION: intra-repository requires disagree on a version:"
    printf '%s\n' "${VERSIONS[@]}" | sort -u | sed 's/^/  /'
    FAILED=1
fi

if [[ "$FAILED" -ne 0 ]]; then
    echo ""
    echo "FAIL: module versions would break an external consumer"
    exit 1
fi

echo "==> OK: all intra-repository requires name a single resolvable release"
