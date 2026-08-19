#!/usr/bin/env bash
#
# Fails when a README presents a removed API as current usage.
#
# The v0.30.0 split deleted pkg/app and its 39 Engine getters, but five module
# READMEs kept showing engine.GetXByName(...) in their "Usage" sections. Nothing
# caught it: the snippets are prose, so neither the compiler nor the linters
# ever read them.
#
# History is exempt. CHANGELOG, MIGRATION, COMPATIBILITY and the ADRs exist to
# name the APIs that were removed, so forbidding the names there would be wrong.
set -euo pipefail

cd "$(dirname "$0")/.."

# APIs deleted in v0.30.0 that must not appear as live example code.
REMOVED='engine\.Get[A-Za-z]+ByName|engine\.GetRestClient|engine\.GetCustomClient|NewAppBuilder|AppBuilder\.|WithInitialization\(|WithCustomClient\('

EXEMPT='^\./(CHANGELOG|MIGRATION|COMPATIBILITY)\.md$|^\./docs/adr/|^\./docs/migration-'

status=0
while IFS= read -r file; do
	if [[ "$file" =~ ^\./(CHANGELOG|MIGRATION|COMPATIBILITY)\.md$ ]] \
		|| [[ "$file" == ./docs/adr/* ]] \
		|| [[ "$file" == ./docs/migration-* ]]; then
		continue
	fi

	if hits=$(grep -nE "$REMOVED" "$file"); then
		echo "FAIL: $file presents an API removed in v0.30.0 as current usage:"
		echo "$hits" | sed 's/^/    /'
		status=1
	fi
done < <(find . -name '*.md' -not -path './node_modules/*' -not -path './.git/*')

if [ "$status" -ne 0 ]; then
	echo ""
	echo "Replace the snippet with the provider API, e.g."
	echo "    client, err := sqsprovider.From(eng, \"orders\")"
	echo "Historical files (CHANGELOG, MIGRATION, COMPATIBILITY, ADRs) are exempt."
	exit 1
fi

echo "OK: no README presents a removed API as current usage"
