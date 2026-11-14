#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
dir=${1:?module directory required};mode=${2:-all}
module=$("$repo/scripts/modules.sh" get "$dir" path)
deps_column=$("$repo/scripts/modules.sh" get "$dir" deps)
track=$("$repo/scripts/modules.sh" get "$dir" track)
name=${dir//\//-}
export GOWORK=off GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
cd "$repo/$dir"
version=$(go env GOVERSION)
if [[ ! "$version" =~ ^go1\.27\.([0-9]+)$ ]] || (( ${BASH_REMATCH[1]:-0} < 4 ));then echo "requires Go 1.27.4 or later 1.27 patch: $version" >&2;exit 1;fi
option(){ "$repo/scripts/modules.sh" override "$dir" "$1"; }
env_value(){ awk -v key="$1" 'BEGIN{printf "%s",ENVIRON[key]}'; }
prefix=$(printf '%s' "$name" | awk '{gsub(/-/,"");print toupper($0)}')
legacy=$(option env-prefix);legacy=${legacy:-$prefix}
setting(){ local value;value=$(env_value "${prefix}_$1");[[ -n "$value" ]] || value=$(env_value "${legacy}_$1");printf '%s' "${value:-$2}"; }
hook(){ local file="$repo/scripts/modules/$name.sh";[[ ! -f "$file" ]] || bash "$file" "$1" "$dir"; }
records(){ awk -F '\t' '!/^#/ && NF' "$repo/modules.tsv"; }
# dep_paths prints the module paths a deps column allows: a catalog dir is
# resolved to its path, an external module path is taken as written.
dep_paths(){
 local item
 [[ "$1" != - && "$1" != stdlib ]] || return 0
 for item in ${1//,/ };do
  if [[ "$item" != . && "${item%%/*}" == *.* ]];then printf '%s\n' "$item";else records | awk -F '\t' -v d="$item" '$1==d{print $2}';fi
 done
}
# The modules a composed consumer may add: the new tools, never the legacy
# core, its families, presets or the harness (answers §12).
composable_paths(){ records | awk -F '\t' '$3=="tool"||$3=="composition"||$3=="source"{print $2}'; }
# boundary OWN ALLOWED checks the module in the current directory against a
# newline-separated list of allowed module paths. Everything is derived from
# go.mod and the build graph; there is no per-module list to keep in sync.
boundary(){
 local own=$1 allowed=$2 imports owners selected edges bad=0
 # 1. Direct imports, tests included, come only from stdlib, itself or allowed.
 imports=$(go list -f '{{range .Imports}}{{println .}}{{end}}{{range .TestImports}}{{println .}}{{end}}{{range .XTestImports}}{{println .}}{{end}}' ./... | awk 'NF && $0!="C"' | sort -u)
 if [[ -n "$imports" ]];then
  # shellcheck disable=SC2086 # import paths contain no whitespace
  owners=$(go list -f '{{.ImportPath}}{{"\t"}}{{with .Module}}{{.Path}}{{end}}' $imports)
  awk -F '\t' -v own="$own" 'FILENAME==ARGV[1]{if(NF)allowed[$1]=1;next} $2!="" && $2!=own && !($2 in allowed){print "forbidden direct import: " $1 " (module " $2 ")" > "/dev/stderr";bad=1}END{exit bad}' <(printf '%s\n' "$allowed") <(printf '%s\n' "$owners") || bad=1
 fi
 # 2. Every selected module is reachable from an allowed one, so an undeclared
 # dependency cannot hide in go.mod as // indirect.
 selected=$(go list -m -f '{{.Path}}' all)
 if [[ "$track" == unreleased || "$track" == independent ]];then
  if printf '%s\n' "$selected" | grep -Fxq github.com/sirupsen/logrus;then echo "$dir: logrus is prohibited in unreleased/independent module graphs" >&2;return 1;fi
 fi
 edges=$(go mod graph)
 awk -v own="$own" '
 FILENAME==ARGV[1]{if(NF)reached[$1]=1;next}
 FILENAME==ARGV[2]{from=$1;to=$2;sub(/@.*$/,"",from);sub(/@.*$/,"",to);if(to!="go" && to!="toolchain"){n++;src[n]=from;dst[n]=to};next}
 FNR==1{do{changed=0;for(i=1;i<=n;i++)if((src[i] in reached) && !(dst[i] in reached)){reached[dst[i]]=1;changed=1}}while(changed)}
 NF && $1!=own && !($1 in reached){print "undeclared module in graph: " $1 > "/dev/stderr";bad=1}
 END{exit bad}' <(printf '%s\n' "$allowed") <(printf '%s\n' "$edges") <(printf '%s\n' "$selected") || bad=1
 # 3. Replaces are exactly the in-repository modules of the graph, each one
 # pointing at its catalog directory. An unused replace is rejected too.
 replaces "$own" "$selected" || bad=1
 return "$bad"
}
replaces(){
 local own=$1 selected=$2 old target dir want expected bad=0
 expected=$(records | awk -F '\t' -v own="$own" 'FILENAME==ARGV[1]{if(NF)sel[$1]=1;next} $2!=own && ($2 in sel){print $2}' <(printf '%s\n' "$selected") -)
 while IFS=$'\t' read -r _ old target;do
  dir=$(records | awk -F '\t' -v p="$old" '$2==p{print $1}')
  if [[ -z "$dir" ]] || ! printf '%s\n' "$expected" | grep -Fxq "$old";then echo "unexpected replace: $old => $target" >&2;bad=1;continue;fi
  want=$repo;[[ "$dir" == . ]] || want="$repo/$dir"
  if [[ "$target" != .* || ! -d "$target" || "$(cd "$target" && pwd)" != "$want" ]];then echo "replace does not point at its catalog dir: $old => $target" >&2;bad=1;fi
 done < <(awk -f "$repo/scripts/modules/metadata.awk" go.mod | awk -F '\t' '$1=="replace"')
 while IFS= read -r old;do
  [[ -n "$old" ]] || continue
  awk -f "$repo/scripts/modules/metadata.awk" go.mod | awk -F '\t' -v p="$old" '$1=="replace" && $2==p{found=1}END{exit !found}' || { echo "missing replace: $old" >&2;bad=1; }
 done <<< "$expected"
 return "$bad"
}
# fuzz_targets ROOT prints the Fuzz functions in a module's test files, nested
# modules, testdata and duplicate " 2" files excluded.
fuzz_targets(){
 (cd "$1" && find . \( -type d \( -name testdata -o -name '.?*' \) -prune \) -o \( -type d ! -path . -exec test -f '{}/go.mod' ';' -prune \) -o \( -type f -name '*_test.go' ! -name '* 2*' -print \) | while IFS= read -r file;do grep -hoE '^func Fuzz[A-Za-z0-9_]+' "$file" || true;done) | sed 's/^func //' | sort -u
}
baseline_fuzz_targets(){
 local ref=$1 nested pathspecs=() nest
 nested=$(git -C "$repo" ls-tree -r --name-only "$ref" -- "$dir" | awk -v d="$dir" '/\/go\.mod$/ && $0!=d "/go.mod"{sub(/\/go\.mod$/,"");print}')
 while IFS= read -r nest;do [[ -z "$nest" ]] || pathspecs+=(":(exclude)$nest");done <<< "$nested"
 git -C "$repo" grep -h -oE '^func Fuzz[A-Za-z0-9_]+' "$ref" -- ":(glob)$dir/**/*_test.go" ":(exclude,glob)**/testdata/**" ${pathspecs[@]+"${pathspecs[@]}"} | sed 's/^func //' | sort -u || true
}
fuzz_inventory(){
 local current baseline missing
 current=$(fuzz_targets "$repo/$dir")
 [[ -n "$current" ]] || { echo "$dir: a production module needs at least one fuzz target" >&2;return 1; }
 [[ -n "${FUZZ_BASELINE_REF:-}" ]] || return 0
 git -C "$repo" rev-parse --verify --quiet "$FUZZ_BASELINE_REF^{commit}" >/dev/null || { echo "FUZZ_BASELINE_REF does not resolve: $FUZZ_BASELINE_REF" >&2;return 1; }
 baseline=$(baseline_fuzz_targets "$FUZZ_BASELINE_REF")
 missing=$(comm -23 <(printf '%s\n' "$baseline" | awk NF) <(printf '%s\n' "$current"))
 [[ -z "$missing" ]] || { printf 'fuzz target removed since %s: %s\n' "$FUZZ_BASELINE_REF" $missing >&2;return 1; }
}
deps(){
 local allowed missing
 allowed=$(dep_paths "$deps_column")
 boundary "$module" "$allowed"
 # A declared dep the module no longer requires leaves the catalog stale.
 missing=$(awk -F '\t' 'FILENAME==ARGV[1]{if(NF)want[$1]=1;next} $1=="require"{delete want[$2]} END{for(m in want)print m}' <(printf '%s\n' "$allowed") <(awk -f "$repo/scripts/modules/metadata.awk" go.mod))
 [[ -z "$missing" ]] || { printf 'catalog deps not required by go.mod: %s\n' $missing >&2;return 1; }
 if [[ "$("$repo/scripts/modules.sh" get "$dir" checks)" == production ]];then fuzz_inventory;fi
 hook deps
 echo "$dir: dependency boundary passes"
}
consumers(){
 local base consumer own allowed exercised=0
 base=$(option consumers-dir);base="$repo/${base:-examples/standalone/$dir}"
 for consumer in minimal composed;do
  cd "$base/$consumer"
  own=$(go list -m -f '{{.Path}}')
  allowed=$(printf '%s\n' "$module";dep_paths "$deps_column")
  if [[ "$consumer" == composed ]];then
   local consumer_extra
   consumer_extra=$("$repo/scripts/modules.sh" consumer-deps "$dir")
   allowed=$(printf '%s\n' "$allowed";composable_paths;printf '%s\n' "$consumer_extra")
  fi
  boundary "$own" "$allowed"
  ! go list -m -f '{{.Path}}' all | grep -Fxq "$module" || exercised=1
  go run .
  go test -race ./...
 done
 cd "$repo/$dir"
 [[ "$exercised" == 1 ]] || { echo "$dir: neither consumer depends on $module" >&2;return 1; }
}
run(){ local label=$1;shift;"$@" 2>&1 | tee "$results/$label.log"; }
quick(){
 run race-default go test -race -count=20 -timeout=120s ./...
 run race-1 env GOMAXPROCS=1 go test -race -count=20 -timeout=120s ./...
 run race-4 env GOMAXPROCS=4 go test -race -count=20 -timeout=120s ./...
 run vet go vet ./...
}
stress(){
 local stress_env;stress_env=$(option stress-env);stress_env=${stress_env:-${prefix}_STRESS}
 if [[ -n "$(option stress-hook)" ]];then env "${prefix}STRESS=1" "$stress_env=1" bash "$repo/scripts/modules/$name.sh" stress "$dir";return;fi
 run stress env "${prefix}STRESS=1" "$stress_env=1" go test -race -run '^TestStress' -v -count=3 -timeout=180s ./...
}
fuzz(){
 local target duration package owner label
 duration=$(setting FUZZ_TIME "$(option fuzz-time)");duration=${duration:-30s}
 : > "$results/fuzz-packages.tsv"
 while IFS= read -r package;do
  go test -list '^Fuzz' "$package" | awk -v p="$package" '/^Fuzz[A-Za-z0-9_]+$/{print $0 "\t" p}' >> "$results/fuzz-packages.tsv"
 done < <(go list -f '{{.ImportPath}}' ./...)
 [[ -s "$results/fuzz-packages.tsv" ]] || { echo "$dir: no fuzz target found" >&2;return 1; }
 while IFS=$'\t' read -r target owner;do
  label="fuzz-$target";[[ ! -e "$results/$label.log" ]] || label="$label-${owner//\//_}"
  run "$label" go test -run '^$' -fuzz "^$target$" -fuzztime="$duration" -parallel=2 "$owner"
 done < "$results/fuzz-packages.tsv"
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
