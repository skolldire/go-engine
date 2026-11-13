#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
dir=${1:?module directory required};mode=${2:-all}
module=$("$repo/scripts/modules.sh" get "$dir" path)
name=${dir//\//-}
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
cd "$repo/$dir"
version=$(go env GOVERSION)
if [[ ! "$version" =~ ^go1\.27\.([0-9]+)$ ]] || (( ${BASH_REMATCH[1]:-0} < 4 ));then echo "requires Go 1.27.4 or later 1.27 patch: $version" >&2;exit 1;fi
options="$repo/scripts/allowlists/$name-options.tsv"
option(){ awk -F '\t' -v k="$1" '$1==k{print $2;exit}' "$options" 2>/dev/null || true; }
env_value(){ awk -v key="$1" 'BEGIN{printf "%s",ENVIRON[key]}'; }
prefix=$(printf '%s' "$name" | awk '{gsub(/-/,"");print toupper($0)}')
legacy=$(option env-prefix);legacy=${legacy:-$prefix}
setting(){ local value;value=$(env_value "${prefix}_$1");[[ -n "$value" ]] || value=$(env_value "${legacy}_$1");printf '%s' "${value:-$2}"; }
hook(){ local file="$repo/scripts/modules/$name.sh";[[ ! -f "$file" ]] || bash "$file" "$1" "$dir"; }
graph(){
 local own=$1 allow=$2 replaces=${3:-} selected packages edges
 selected=$(go list -m -f '{{.Path}} {{.Version}}' all)
 [[ -f "$allow" ]] || allow=/dev/null
 awk -v own="$own" 'FILENAME==ARGV[1]{if(NF && !/^#/){allowed[$1]=$2;expected[$1]=1};next} NF{if($1==own)next;if(!($1 in allowed)){print "forbidden module: " $1 > "/dev/stderr";bad=1}else if(allowed[$1]!="" && $2!=allowed[$1]){print "wrong selected version: " $1 " " $2 > "/dev/stderr";bad=1};delete expected[$1]}END{for(m in expected){print "missing declared dependency: " m > "/dev/stderr";bad=1};exit bad}' "$allow" <(printf '%s\n' "$selected")
 packages=$(go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
 awk -v own="$own" 'FILENAME==ARGV[1]{if(NF && !/^#/)allowed[$1]=1;next} NF && $1!=own && !($1 in allowed){print "forbidden package module: " $1 > "/dev/stderr";bad=1}END{exit bad}' "$allow" <(printf '%s\n' "$packages")
 edges=$(go mod graph)
 awk -v own="$own" 'FILENAME==ARGV[1]{if(NF && !/^#/)allowed[$1]=1;next}{for(i=1;i<=NF;i++){node=$i;sub(/@.*$/,"",node);if(node!=own && node!="go" && node!="toolchain" && !(node in allowed)){print "forbidden graph node: " $i > "/dev/stderr";bad=1}}}END{exit bad}' "$allow" <(printf '%s\n' "$edges")
 if [[ -n "$replaces" ]];then
  selected=$(awk -f "$repo/scripts/modules/metadata.awk" go.mod | awk -F '\t' '$1=="replace"{print $3}')
  awk 'FILENAME==ARGV[1]{if(NF && !/^#/)expected[$1]=1;next}NF{if(!($1 in expected)){print "unexpected replace: " $1 > "/dev/stderr";bad=1};delete expected[$1]}END{for(p in expected){print "missing replace: " p > "/dev/stderr";bad=1};exit bad}' "$replaces" <(printf '%s\n' "$selected")
 fi
}
deps(){
 graph "$module" "$repo/scripts/allowlists/$name-modules.txt"
 local rules="$repo/scripts/allowlists/$name-forbidden-imports.txt" expression status
 if [[ -f "$rules" ]];then while IFS= read -r expression;do
  [[ -n "$expression" && "$expression" != \#* ]] || continue
  if grep -RnE --include='*.go' --exclude='* 2*' "$expression" "$repo/$dir";then echo "forbidden direct import in $dir" >&2;return 1;else status=$?;[[ "$status" == 1 ]] || return "$status";fi
 done < "$rules";fi
 local importer_file="$repo/scripts/allowlists/$name-forbidden-importers.txt" importer dependencies
 if [[ -f "$importer_file" ]];then while IFS= read -r importer;do
  [[ -f "$repo/$importer/go.mod" ]] || continue
  dependencies=$(cd "$repo/$importer" && go list -deps -test -f '{{with .Module}}{{.Path}}{{end}}' ./...)
  if printf '%s\n' "$dependencies" | grep -Fx "$module" >/dev/null;then echo "$importer must not import $module" >&2;return 1;fi
 done < "$importer_file";fi
 hook deps
 echo "$dir: dependency boundary passes"
}
consumers(){
 local base="$repo/examples/standalone/$dir" consumer own override
 override="$repo/scripts/allowlists/$name-consumers-dir.txt"
 [[ ! -f "$override" ]] || base="$repo/$(awk 'NF && !/^#/{print;exit}' "$override")"
 for consumer in minimal composed;do
  cd "$base/$consumer"
  own=$(go list -m -f '{{.Path}}')
  graph "$own" "$repo/scripts/allowlists/$name-$consumer-modules.txt" "$repo/scripts/allowlists/$name-$consumer-replaces.txt"
  go run .
  go test -race ./...
 done
 cd "$repo/$dir"
}
run(){ local label=$1;shift;"$@" 2>&1 | tee "$results/$label.log"; }
quick(){
 run race-default go test -race -count=20 -timeout=120s ./...
 run race-1 env GOMAXPROCS=1 go test -race -count=20 -timeout=120s ./...
 run race-4 env GOMAXPROCS=4 go test -race -count=20 -timeout=120s ./...
 run vet go vet ./...
}
stress(){
 local stress_env;stress_env=$(option stress-env);stress_env=${stress_env:-${prefix}STRESS}
 if [[ -n "$(option stress-hook)" ]];then env "${prefix}STRESS=1" "$stress_env=1" bash "$repo/scripts/modules/$name.sh" stress "$dir";return;fi
 run stress env "${prefix}STRESS=1" "$stress_env=1" go test -race -run '^TestStress' -v -count=3 -timeout=180s ./...
}
fuzz(){
 local target discovered duration package matches owner
 duration=$(setting FUZZ_TIME "$(option fuzz-time)");duration=${duration:-30s}
 : > "$results/fuzz-packages.tsv"
 while IFS= read -r package;do
  discovered=$(go test -list '^Fuzz' "$package")
  printf '%s\n' "$discovered" | awk -v p="$package" '/^Fuzz[A-Za-z0-9_]+$/{print $0 "\t" p}' >> "$results/fuzz-packages.tsv"
 done < <(go list -f '{{.ImportPath}}' ./...)
 while IFS= read -r target;do
  [[ -n "$target" && "$target" != \#* ]] || continue
  [[ "$target" =~ ^Fuzz[A-Za-z0-9_]+$ ]] || { echo 'invalid fuzz target' >&2;return 1; }
  matches=$(awk -F '\t' -v t="$target" '$1==t{n++}END{print n+0}' "$results/fuzz-packages.tsv")
  [[ "$matches" == 1 ]] || { echo "missing or ambiguous fuzz target: $target ($matches matches)" >&2;return 1; }
 done < "$repo/scripts/allowlists/$name-fuzz.txt"
 while IFS= read -r target;do
  [[ -n "$target" && "$target" != \#* ]] || continue
  owner=$(awk -F '\t' -v t="$target" '$1==t{print $2}' "$results/fuzz-packages.tsv")
  run "fuzz-$target" go test -run '^$' -fuzz "^$target$" -fuzztime="$duration" -parallel=2 "$owner"
 done < "$repo/scripts/allowlists/$name-fuzz.txt"
}
bench(){ local duration;duration=$(setting BENCH_TIME "$(option bench-time)");run benchmarks go test -run '^$' -bench . -benchmem -benchtime="${duration:-500ms}" -count=3 -timeout=180s ./...; }
case "$mode" in
 deps)deps;;consumers)consumers;;
 quick|stress|fuzz|bench|all)
  results=$(setting RESULTS_DIR '');results=${results:-$(mktemp -d "${TMPDIR:-/tmp}/$name-validation.XXXXXX")};mkdir -p "$results";results=$(cd "$results" && pwd);export MODULE_RESULTS_DIR="$results"
  go version | tee "$results/environment.log";go env -json GOOS GOARCH GOVERSION CGO_ENABLED >> "$results/environment.log"
  case "$mode" in all)quick;stress;fuzz;bench;;*)"$mode";;esac
  printf 'results=%s\n' "$results";;
 *)[[ -f "$repo/scripts/modules/$name.sh" ]] || { echo "unknown mode: $mode" >&2;exit 2; };hook "$mode";;
esac
