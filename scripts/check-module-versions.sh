#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
allow_current=0
case "${1:-}" in '') ;; --allow-current)allow_current=1;;*)echo 'usage: check-module-versions.sh [--allow-current]' >&2;exit 2;;esac
./scripts/modules.sh validate
bad=0
while IFS= read -r dir;do
 owner_track=$(./scripts/modules.sh get "$dir" track)
 [[ "$owner_track" != none ]] || continue
 while IFS=$'\t' read -r record path version;do
  [[ "$record" == require ]] || continue
  target=$(awk -F '\t' -v p="$path" '!/^#/ && $2==p{print $1}' modules.tsv)
  if [[ -z "$target" ]];then
   root_path=$(./scripts/modules.sh get . path)
   case "$path" in "$root_path"/*)echo "VIOLATION: $dir requires uncatalogued sibling $path" >&2;bad=1;;esac
   continue
  fi
  track=$(./scripts/modules.sh get "$target" track)
  expected=$(./scripts/modules.sh get "$target" version)
  if [[ "$track" == unreleased ]];then
   if [[ "$owner_track" != unreleased || "$version" != v0.0.0 ]];then echo "VIOLATION: $dir requires unreleased $path at $version" >&2;bad=1;continue;fi
   replacement=$(awk -f scripts/modules/metadata.awk "$dir/go.mod" | awk -F '\t' -v p="$path" '$1=="replace" && $2==p{print $3}')
   case "$replacement" in ..|.|../*|./*)
    actual=$(cd "$dir" && cd "$replacement" && pwd -P) || actual=''
    desired=$(cd "$target" && pwd -P)
    [[ "$actual" == "$desired" ]] || { echo "VIOLATION: $dir replacement for $path points elsewhere" >&2;bad=1; } ;;
    *)echo "VIOLATION: $dir unreleased sibling $path needs a relative replace" >&2;bad=1;;
   esac
  elif [[ "$track" == none || "$version" != "$expected" ]];then
   echo "VIOLATION: $dir requires $path at $version, expected approved $expected ($track)" >&2;bad=1
  fi
 done < <(awk -f scripts/modules/metadata.awk "$dir/go.mod")
done < <(./scripts/modules.sh dirs)
version=$(./scripts/modules.sh get . version)
latest=$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | awk '/^v[0-9]+\.[0-9]+\.[0-9]+$/ && !found++{print}')
if [[ -n "$latest" ]];then
 newer=$(awk -v a="$version" -v b="$latest" 'BEGIN{sub(/^v/,"",a);sub(/^v/,"",b);split(a,x,".");split(b,y,".");for(i=1;i<=3;i++){if(length(x[i])!=length(y[i])){print(length(x[i])>length(y[i]));exit};if("x" x[i]!="x" y[i]){print("x" x[i]>"x" y[i]);exit}}print 0}')
 if [[ "$allow_current" == 1 && "$version" == "$latest" ]];then
  while IFS= read -r dir;do tag="$version";[[ "$dir" == . ]] || tag="$dir/$version"
   ref=$(git rev-parse "$tag^{commit}" 2>/dev/null || true)
   [[ "$ref" == "$(git rev-parse HEAD)" ]] || { echo "VIOLATION: post-tag verification needs $tag at HEAD" >&2;bad=1; }
  done < <(./scripts/modules.sh dirs --track coordinated)
 elif [[ "$newer" != 1 ]];then echo "VIOLATION: coordinated $version must be ahead of $latest (strict preflight)" >&2;bad=1;fi
fi
[[ "$bad" == 0 ]] || exit 1
echo 'module versions: catalog tracks and sibling requirements pass'
