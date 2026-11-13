#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
mode=${1:-unit}
results=${WAVE0_RESULTS_DIR:-$repo/release-evidence/wave0}
mkdir -p "$results";results=$(cd "$results" && pwd)
export GOTOOLCHAIN=local GOPROXY=off GOFLAGS=-mod=readonly
workspace=$(mktemp -d "${TMPDIR:-/tmp}/wave0-test.XXXXXX")
trap 'rm -rf "$workspace"' EXIT
snapshot() { 
 mkdir -p "$1"
 git -C "$repo" archive HEAD | tar -x -C "$1"
 cp "$repo/modules.tsv" "$repo/Makefile" "$repo/go.work" "$1/"
 cp -R "$repo/scripts/." "$1/scripts/"
 cp -R "$repo/.github/workflows/." "$1/.github/workflows/"
}
old_run() { 
 local root=$1 dir=$2 mode=$3 family=$2 oldmode
 oldmode=$mode
 [[ "$dir" != config/viper ]] || family=config
 case "$mode" in consumers)oldmode=module;;quick|fuzz)oldmode=production;;esac
 case "$mode" in quick|fuzz) (cd "$root" && CONFIG_MODULE="$dir" CONFIG_FUZZ_TIME=1x CONCURRENCY_FUZZ_TIME=1x CLIENT_FUZZ_TIME=1x RETRY_FUZZ_TIME=1x CIRCUITBREAKER_FUZZ_TIME=1x RATELIMIT_FUZZ_TIME=1x RESILIENCE_FUZZ_TIME=1x bash "scripts/check-$family-$oldmode.sh" "$mode");;
 *) (cd "$root" && bash "scripts/check-$family-$oldmode.sh");;esac
}
new_run() { (cd "$1" && bash scripts/check-module.sh "$2" "$3");}
expect_fail() { local label=$1;shift;if "$@" > "$results/$label.log" 2>&1;then echo "FAIL: $label unexpectedly passed" >&2;return 1;fi;echo "PASS rejection $label";}
case "$mode" in
 unit)
 snapshot "$workspace/repo";cd "$workspace/repo"
 ./scripts/modules.sh validate
 ./scripts/modules.sh lint-lists
 json=$(./scripts/modules.sh matrix)
 cat > "$workspace/json.go" <<'GO'
package main
import("encoding/json";"fmt";"os";"strings")
func main() { var m struct{Include []struct{Dir,Name string}};if err:=json.Unmarshal([]byte(os.Args[1]),&m);err!=nil{panic(err)};if len(m.Include)==0{panic("empty matrix")};seen:=map[string]bool{};for _,row:=range m.Include{if seen[row.Dir]||row.Name!=strings.ReplaceAll(row.Dir,"/","-"){panic("bad matrix")};seen[row.Dir]=true};fmt.Println("matrix decoded by encoding/json:",len(m.Include))}
GO
 (cd "$workspace" && GOWORK=off GO111MODULE=off go run json.go "$json")
 cp modules.tsv "$workspace/catalog"
 printf '\nunknown\texample.com/unknown\ttool\tunreleased\t-\tbasic\n' >> modules.tsv
 expect_fail missing-module ./scripts/modules.sh validate
 cp "$workspace/catalog" modules.tsv
 printf '# no modules\n' > modules.tsv
 expect_fail empty-matrix ./scripts/modules.sh matrix
 cp "$workspace/catalog" modules.tsv
 printf '\nMODULES := . concurrency\n' >> Makefile
 expect_fail hardcoded-list ./scripts/modules.sh lint-lists
 cp "$repo/Makefile" Makefile
 cp .github/workflows/ci.yml "$workspace/ci.yml"
 printf '\nstrategy:\n  matrix:\n    dir:\n      - concurrency\n      - retry\n' >> .github/workflows/ci.yml
 expect_fail hardcoded-yaml-matrix ./scripts/modules.sh lint-lists
 cp "$workspace/ci.yml" .github/workflows/ci.yml
 mkdir orphan;printf 'module example.com/orphan\n\ngo 1.27.4\n' > orphan/go.mod
 expect_fail orphan-module ./scripts/modules.sh validate
 rm -rf orphan
 cp aws/go.mod "$workspace/aws.mod"
 printf '\nrequire github.com/skolldire/go-engine/client v0.0.0\n' >> aws/go.mod
 expect_fail published-to-unreleased ./scripts/modules.sh validate
 cp "$workspace/aws.mod" aws/go.mod
 bash -n scripts/modules.sh scripts/check-module.sh scripts/check-module-versions.sh scripts/sync-module-versions.sh scripts/release-plan.sh
 echo 'unit and matrix parser checks passed' ;;
 equivalence)
 old=${WAVE0_OLD_ROOT:-/private/tmp/wave0-original}
 [[ -f "$old/scripts/check-client-deps.sh" ]] || { echo 'original snapshot missing' >&2;exit 1; }
 for dir in $("$repo/scripts/modules.sh" dirs --checks production);do
  name=${dir//\//-}
  for check in deps consumers quick;do
   old_run "$old" "$dir" "$check" > "$results/equivalence-old-$name-$check.log" 2>&1
   new_run "$repo" "$dir" "$check" > "$results/equivalence-new-$name-$check.log" 2>&1
   echo "PASS equivalence $dir $check"
  done
 done ;;
 mutation)
 old=${WAVE0_OLD_ROOT:-/private/tmp/wave0-original}
 snapshot "$workspace/new"
 mkdir "$workspace/old";cp -R "$old/." "$workspace/old/"
 old="$workspace/old";new="$workspace/new"
 for root in "$old" "$new";do mkdir -p "$root/testdata/forbidden";printf 'module example.com/forbidden\n\ngo 1.27.4\n' > "$root/testdata/forbidden/go.mod";done
 for dir in $("$repo/scripts/modules.sh" dirs --checks production);do
  name=${dir//\//-};family=$dir;[[ "$dir" != config/viper ]] || family=config
  for target in module minimal composed;do
   location="$dir";check=deps;[[ "$target" == module ]] || { location="examples/standalone/$family/$target";check=consumers; }
   cp "$new/$location/go.mod" "$workspace/mod"
   for root in "$old" "$new";do (cd "$root/$location" && GOWORK=off go mod edit -require=example.com/forbidden@v0.0.0 -replace="example.com/forbidden=$root/testdata/forbidden");done
   expect_fail "mutation-new-$name-$target" new_run "$new" "$dir" "$check"
   if old_run "$old" "$dir" "$check" > "$results/mutation-old-$name-$target.log" 2>&1;then echo "IMPROVEMENT: old $dir $target allowed forbidden dependency";else echo "PASS old rejection $dir $target";fi
   for root in "$old" "$new";do cp "$workspace/mod" "$root/$location/go.mod";done
  done
  location="examples/standalone/$family/minimal";cp "$new/$location/go.mod" "$workspace/mod"
  for root in "$old" "$new";do (cd "$root/$location" && GOWORK=off go mod edit -replace="example.com/unused=$root/testdata/forbidden");done
  expect_fail "mutation-new-$name-replace" new_run "$new" "$dir" consumers
  if old_run "$old" "$dir" consumers > "$results/mutation-old-$name-replace.log" 2>&1;then echo "IMPROVEMENT: old $dir ignored unused unexpected replace";else echo "PASS old replacement rejection $dir";fi
  for root in "$old" "$new";do cp "$workspace/mod" "$root/$location/go.mod";done
  cp "$new/scripts/allowlists/$name-fuzz.txt" "$workspace/fuzz"
  printf 'FuzzWave0Missing\n' > "$new/scripts/allowlists/$name-fuzz.txt"
  expect_fail "mutation-new-$name-fuzz" new_run "$new" "$dir" fuzz
  file="$old/scripts/check-$family-production.sh";cp "$file" "$workspace/old-production"
  awk '/^fuzz\(\)[[:space:]]*\{/ {print;print " go test -run '\''^$'\'' -fuzz '\''^FuzzWave0Missing$'\'' -fuzztime=1x; return";next} {print}' "$file" > "$workspace/edited"
  cp "$workspace/edited" "$file"
  if old_run "$old" "$dir" fuzz > "$results/mutation-old-$name-fuzz.log" 2>&1;then echo "IMPROVEMENT: old $dir treats absent fuzz target as success";else echo "PASS old fuzz rejection $dir";fi
  cp "$workspace/old-production" "$file";cp "$workspace/fuzz" "$new/scripts/allowlists/$name-fuzz.txt"
 done ;;
 dry-run)
 git clone --quiet --no-hardlinks "$repo" "$workspace/clone"
 git -C "$workspace/clone" remote remove origin
 cd "$workspace/clone"
 cp "$repo/modules.tsv" "$repo/Makefile" "$repo/go.work" .
 cp -R "$repo/scripts/." scripts/
 ./scripts/check-module-versions.sh
 ./scripts/release-plan.sh > "$results/release-plan.log"
 awk -F '\t' '!/^#/ && $4=="coordinated"{print ($1=="."?$5:$1 "/" $5)}' modules.tsv > "$workspace/expected"
 [[ "$(cat "$workspace/expected")" == "$(cat "$results/release-plan.log")" ]] || exit 1
 [[ "$(awk 'END{print NR}' "$workspace/expected")" == 10 ]] || exit 1
 while IFS= read -r tag;do git tag "$tag";done < "$workspace/expected"
 ./scripts/check-module-versions.sh --allow-current
 expect_fail dry-run-strict-preflight ./scripts/check-module-versions.sh
 while IFS= read -r dir;do
  [[ -z "$(git tag --list "$dir/v*")" ]] || { echo "unexpected unreleased tag: $dir" >&2;exit 1; }
 done < <(./scripts/modules.sh dirs --track unreleased)
 cp modules.tsv "$workspace/catalog"
 awk -F '\t' -v OFS='\t' '$1=="resilience"{$4="coordinated";$5="v0.30.0"}{print}' modules.tsv > "$workspace/promoted"
 cp "$workspace/promoted" modules.tsv
 expect_fail unapproved-promotion ./scripts/modules.sh validate
 cp "$workspace/catalog" modules.tsv
 ./scripts/sync-module-versions.sh v0.31.0
 ./scripts/check-module-versions.sh
 for dir in $(./scripts/modules.sh dirs --track unreleased);do cmp "$dir/go.mod" "$repo/$dir/go.mod";done
 cp modules.tsv "$workspace/coordinated"
 awk -F '\t' -v OFS='\t' '$1=="client"{$4="independent";$5="v0.1.0"}{print}' modules.tsv > "$workspace/independent"
 cp "$workspace/independent" modules.tsv
 expect_fail independent-wrong-require ./scripts/check-module-versions.sh
 (cd resilience && GOWORK=off go mod edit -require=github.com/skolldire/go-engine/client@v0.1.0)
 ./scripts/check-module-versions.sh
 [[ "$(./scripts/release-plan.sh --track independent)" == client/v0.1.0 ]] || exit 1
 printf '\nreplace github.com/skolldire/go-engine v0.30.0 => ../\n' >> aws/go.mod
 ./scripts/sync-module-versions.sh v0.32.0
 grep -F 'replace github.com/skolldire/go-engine v0.30.0 => ../' aws/go.mod >/dev/null
 [[ "$(./scripts/modules.sh get client version)" == v0.1.0 ]] || exit 1
 ./scripts/check-module-versions.sh
 echo 'independent approval/require matching and print-only plan pass in clone'
 echo 'dry-run: ten local tags only in disposable clone; strict/post-tag gates and coordinated-only sync pass' ;;
 fixture)
 snapshot "$workspace/repo";cd "$workspace/repo"
 mkdir -p wave0fixture examples/standalone/wave0fixture/minimal examples/standalone/wave0fixture/composed
 printf 'module example.com/wave0fixture\n\ngo 1.27.4\n' > wave0fixture/go.mod
 printf 'package wave0fixture\nfunc Value()int{return 42}\n' > wave0fixture/value.go
 for consumer in minimal composed;do
  printf 'module example.com/wave0fixture-%s\n\ngo 1.27.4\nrequire example.com/wave0fixture v0.0.0\nreplace example.com/wave0fixture => ../../../../wave0fixture\n' "$consumer" > "examples/standalone/wave0fixture/$consumer/go.mod"
  printf 'package main\nimport("example.com/wave0fixture";"fmt")\nfunc main() { fmt.Println(wave0fixture.Value())}\n' > "examples/standalone/wave0fixture/$consumer/main.go"
  printf 'example.com/wave0fixture v0.0.0\n' > "scripts/allowlists/wave0fixture-$consumer-modules.txt"
  printf '../../../../wave0fixture\n' > "scripts/allowlists/wave0fixture-$consumer-replaces.txt"
 done
 : > scripts/allowlists/wave0fixture-modules.txt
 : > scripts/allowlists/wave0fixture-fuzz.txt
 printf 'wave0fixture\texample.com/wave0fixture\ttool\tunreleased\t-\tproduction\n' >> modules.tsv
 go work edit -use ./wave0fixture
 ./scripts/modules.sh validate
 ./scripts/modules.sh lint-lists
 make lint-arch
 make check-all-modules
 cmp Makefile "$repo/Makefile"
 echo 'fixture discovered through catalog/data/workspace only; no script, workflow or Makefile edits' ;;
 *)echo 'unit|equivalence|mutation|dry-run|fixture' >&2;exit 2;;
esac
