#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
for directory in config config/viper; do
 cd "$repo/$directory"
 name=config
 [[ "$directory" == config/viper ]] && name=config-viper
 expected=$(cat "$repo/scripts/allowlists/$name-modules.txt")
 selected=$(go list -m -f '{{if .Version}}{{.Path}} {{.Version}}{{end}}' all | sed '/^$/d' | sed '\|^github.com/skolldire/go-engine/config |d' | sort)
 [[ "$selected" == "$expected" ]] || { echo "unexpected selected module graph: $directory"; diff -u <(printf '%s\n' "$expected") <(printf '%s\n' "$selected") || true; exit 1; }
 packages=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
 while IFS= read -r module; do
  case "$module" in ''|github.com/skolldire/go-engine/config|github.com/skolldire/go-engine/config/viper) ;;
   *) awk -v module="$module" '$1==module {found=1} END {exit !found}' "$repo/scripts/allowlists/$name-modules.txt" || { echo "forbidden dependency: $module";exit 1; } ;;
  esac
 done <<< "$packages"
 echo "$directory: selected versions and runtime/test imports match allowlist"
done

scan(){
 if command -v rg >/dev/null; then
  rg -n "$1" "$repo/config" --glob '*.go' --glob '!* 2*'
 else
  grep -RnE --include='*.go' --exclude='* 2*' "$1" "$repo/config"
 fi
}
for expression in '"github.com/mitchellh/mapstructure' '"(go\.yaml\.in/|gopkg\.in/yaml|github\.com/pelletier/go-toml)'; do
 if scan "$expression"; then echo 'forbidden direct import in new config modules';exit 1
 else result=$?;[[ "$result" == 1 ]] || exit "$result";fi
done
