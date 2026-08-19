#!/usr/bin/env bash
#
# Fails when documentation or example code presents an API removed in v0.30.0
# as if it still existed.
#
# Why this exists: the split deleted pkg/app and its Engine getters, but the
# names lived on in prose. Nothing caught it, because snippets in Markdown and
# text in Go comments are invisible to the compiler and to golangci-lint. Five
# module READMEs, CONTRIBUTING and a package Example were all still showing them.
#
# Scope: Markdown *and* Go files. Restricting the search to Markdown was the
# first version's blind spot -- it reported OK while pkg/core/client/
# example_test.go presented NewAppBuilder as current usage. Scanning Go is safe
# precisely because these symbols no longer exist: live code referencing them
# would not compile, so every hit is necessarily a comment or a string.
#
# Matching is by whole symbol, with or without a call's parentheses. Requiring
# "(" was the second blind spot: CONTRIBUTING named WithCustomClient in prose.
set -euo pipefail

cd "$(dirname "$0")/.."

# Symbols removed in v0.30.0. Verified absent from compiled code; see the header.
REMOVED_SYMBOLS=(
	NewAppBuilder
	AppBuilder
	ServiceRegistry
	ConfigRegistry
	WithCustomClient
	GetCustomClient
	WithInitialization
	WithDynamicConfig
	GetCloudClient
	GetRestClient
	GrpcServer
)

# Engine getters were generated per client, so they are matched by shape.
REMOVED_PATTERNS=(
	'engine\.Get[A-Za-z]+ByName'
	'engine\.Get[A-Za-z]+Client\b'
)

# An escape hatch for text that is deliberately about the past. It exempts the
# single line it appears on and must carry a reason, so an exemption is a
# decision on the record rather than a silent gap that widens over time.
ALLOW_MARKER='removed-api-ok'

# History has to be free to name what it removed: changelogs, migration guides,
# ADRs and the audit plans all exist to describe APIs that are gone.
is_historical() {
	case "$1" in
	./CHANGELOG.md | ./MIGRATION.md | ./COMPATIBILITY.md) return 0 ;;
	./docs/adr/* | ./docs/migration-* | ./docs/plan-auditoria-*) return 0 ;;
	./scripts/check-doc-apis.sh) return 0 ;;
	./scripts/testdata/*) return 0 ;;
	esac
	return 1
}

scan() {
	local root="$1" status=0 expr
	expr=$(
		IFS='|'
		echo "\\b(${REMOVED_SYMBOLS[*]})\\b"
	)
	for p in "${REMOVED_PATTERNS[@]}"; do
		expr="$expr|$p"
	done

	while IFS= read -r file; do
		is_historical "$file" && continue

		local hits
		hits=$(grep -nE "$expr" "$file" || true)
		[ -z "$hits" ] && continue

		# Drop lines that carry an explicit, reasoned exemption.
		hits=$(echo "$hits" | grep -v "$ALLOW_MARKER" || true)
		[ -z "$hits" ] && continue

		echo "FAIL: $file presents an API removed in v0.30.0 as current usage:"
		echo "$hits" | sed 's/^/    /'
		status=1
	done < <(find "$root" \( -name '*.md' -o -name '*.go' \) \
		-not -path '*/node_modules/*' -not -path '*/.git/*' | sort)

	return $status
}

# --- self test -------------------------------------------------------------
#
# A gate nobody tests is a gate nobody can trust: the first version of this
# script reported OK against a tree that had four stale examples in it. These
# fixtures prove it fails when it should and passes when it should.
if [ "${1:-}" = "--self-test" ]; then
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT

	mkdir -p "$tmp/clean" "$tmp/dirty"

	cat >"$tmp/clean/ok.md" <<'EOF'
client, err := sqsprovider.From(eng, "orders")
EOF
	# The marker exempts the line it sits on, and only that line: an exemption
	# has to point at the text it excuses, or it silently widens over time.
	cat >"$tmp/clean/ok.go" <<'EOF'
// Superseded by engine.WithProvider; the old entry point was AppBuilder. removed-api-ok: explains what replaced it.
EOF

	cat >"$tmp/dirty/readme.md" <<'EOF'
Inject it via WithCustomClient and read it back later.
EOF
	cat >"$tmp/dirty/example_test.go" <<'EOF'
// engine, _ := app.NewAppBuilder().Build()
EOF
	cat >"$tmp/dirty/getter.md" <<'EOF'
q := engine.GetSQSClientByName("orders")
EOF
	cat >"$tmp/dirty/field.md" <<'EOF'
grpcSrv := engine.GrpcServer
EOF

	fails=0

	if scan "$tmp/clean" >/dev/null 2>&1; then
		echo "self-test: clean fixtures pass ...... ok"
	else
		echo "self-test: clean fixtures pass ...... FAILED (false positive)"
		scan "$tmp/clean" || true
		fails=1
	fi

	for f in readme.md example_test.go getter.md field.md; do
		mkdir -p "$tmp/one"
		cp "$tmp/dirty/$f" "$tmp/one/"
		if scan "$tmp/one" >/dev/null 2>&1; then
			echo "self-test: detects $f ...... FAILED (false negative)"
			fails=1
		else
			echo "self-test: detects $f ...... ok"
		fi
		rm -rf "$tmp/one"
	done

	[ "$fails" -eq 0 ] || {
		echo "self-test FAILED"
		exit 1
	}
	echo "self-test passed"
	exit 0
fi

# --- repository scan -------------------------------------------------------
if scan "."; then
	echo "OK: no documentation or example presents an API removed in v0.30.0"
else
	echo ""
	echo "Replace the snippet with the current API, for example:"
	echo "    client, err := sqsprovider.From(eng, \"orders\")"
	echo "    srv, err := grpcsrvprovider.From(eng)"
	echo ""
	echo "If the text is deliberately about the past, mark the line:"
	echo "    // ... $ALLOW_MARKER: <reason>"
	echo "CHANGELOG, MIGRATION, COMPATIBILITY and the ADRs are exempt already."
	exit 1
fi
