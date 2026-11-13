#!/usr/bin/env bash
set -euo pipefail
version=${1:-}
[[ "$version" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo 'usage: sync-module-versions.sh vX.Y.Z' >&2;exit 2; }
repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
./scripts/modules.sh validate
while IFS= read -r dir;do
 output=$(mktemp)
 awk -F '\t' -v version="$version" '
 FILENAME==ARGV[1]{if(!/^#/ && $4=="coordinated")coordinated[$2]=1;next}
 {
  text=$0;gsub(/^[[:space:]]+|[[:space:]]+$/,"",text)
  if(text ~ /^require[[:space:]]+\(/)block=1
  if(text ~ /^\)/)block=0
  if(!block && text !~ /^require[[:space:]]+/){print;next}
  n=split($0,words,/[[:space:]]+/)
  for(i=1;i<=n;i++)if(words[i] in coordinated){pattern=words[i];gsub(/\./,"\\.",pattern);sub(pattern "[[:space:]]+v[^[:space:])]+",words[i] " " version)}
  print
 }' modules.tsv "$dir/go.mod" > "$output"
 cat "$output" > "$dir/go.mod";rm -f "$output"
done < <(./scripts/modules.sh dirs --track coordinated)
output=$(mktemp)
awk -F '\t' -v OFS='\t' -v v="$version" '!/^#/ && $4=="coordinated"{$5=v} {print}' modules.tsv > "$output"
cat "$output" > modules.tsv;rm -f "$output"
echo "coordinated catalog and coordinated-to-coordinated requires set to $version"
