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

# e2e is a test harness, never tagged and never installed by anyone, so it is
# free to resolve its siblings through replace directives.
for gomod in $(find . -name go.mod -not -path './.git/*' -not -path './example/*' | sort); do
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

# The declared version is what will be tagged, so it must be ahead of what is
# already released. Without this, the modules can say v0.30.0 while the version
# workflow computes v0.21.0 from the last root tag, and the release publishes a
# tag no module requires.
DECLARED=$(printf '%s\n' "${VERSIONS[@]:-}" | sort -u | head -n 1)
LATEST=$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname 2>/dev/null | head -n 1)

if [[ -n "${DECLARED:-}" && -n "${LATEST:-}" ]]; then
    NEWEST=$(printf '%s\n%s\n' "$DECLARED" "$LATEST" | sort -V | tail -n 1)
    if [[ "$DECLARED" == "$LATEST" || "$NEWEST" != "$DECLARED" ]]; then
        echo "VIOLATION: modules declare $DECLARED but $LATEST is already tagged"
        echo "  the release must publish a version newer than the last one;"
        echo "  run scripts/sync-module-versions.sh with the version you intend to tag"
        FAILED=1
    fi
fi

if [[ "$FAILED" -ne 0 ]]; then
    echo ""
    echo "FAIL: module versions would break an external consumer"
    exit 1
fi

echo "==> OK: modules declare ${DECLARED:-a single version}, ahead of ${LATEST:-no tag}"
