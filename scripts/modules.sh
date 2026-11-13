#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo"
catalog="$repo/modules.tsv"
records(){ awk -F '\t' '!/^#/ && NF {print}' "$catalog"; }
schema(){
 awk -F '\t' '
 function fail(message){print "catalog line " NR ": " message > "/dev/stderr";bad=1}
 /^#/ || !NF {next}
 NF!=7 {fail("expected seven tab-separated columns");next}
 {
  rows++
  if($1!="." && ($1 !~ /^[A-Za-z0-9_.-]+(\/[A-Za-z0-9_.-]+)*$/ || $1 ~ /(^|\/)\.\.?($|\/)/)) fail("invalid dir")
  if($2 !~ /^[A-Za-z0-9_.-]+(\/[A-Za-z0-9_.-]+)*$/) fail("invalid module path")
  if($3 !~ /^(core|tool|composition|source|family|preset|harness)$/) fail("invalid kind")
  if($4 !~ /^(coordinated|independent|unreleased|none)$/) fail("invalid track")
  if($6 !~ /^(production|basic|none)$/) fail("invalid checks")
  if($7!="-" && $7!="stdlib" && $7 !~ /^[A-Za-z0-9_.~-]+(\/[A-Za-z0-9_.~-]+)*(,[A-Za-z0-9_.~-]+(\/[A-Za-z0-9_.~-]+)*)*$/) fail("invalid deps")
  if($6=="production" && $7=="-") fail("production module must declare deps (stdlib or a list)")
  if(($4=="unreleased" || $4=="none") && $5!="-") fail("unreleased/none version must be -")
  if(($4=="coordinated" || $4=="independent") && $5 !~ /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/) fail("version must be vX.Y.Z")
  if($4=="coordinated") {if(coordinated!="" && coordinated!=$5) fail("coordinated versions disagree");coordinated=$5}
  name=$1;gsub(/\//,"-",name)
  if(dirs[$1]++) fail("duplicate dir " $1)
  if(paths[$2]++) fail("duplicate path " $2)
  if(names[name]++) fail("duplicate name " name)
 }
 END{if(rows==0) fail("empty catalog");exit bad}' "$catalog"
}
column(){ case "$1" in dir)echo 1;;path)echo 2;;kind)echo 3;;track)echo 4;;version)echo 5;;checks)echo 6;;deps)echo 7;;*)echo 'unknown column' >&2;return 2;;esac; }
scan_mods(){
 local base=$1 entry relative
 [[ ! -f "$base/go.mod" ]] || printf '%s\n' "${base#./}"
 for entry in "$base"/*/ "$base"/.[!.]*/ "$base"/..?*/;do
  [[ -d "$entry" ]] || continue
  entry=${entry%/};relative=${entry#./}
  case "$relative/" in .git/*|*/.git/*|testdata/*|*/testdata/*|examples/standalone/*|*' 2'*)continue;;esac
  scan_mods "$entry"
 done
}
validate(){
 local bad=0 dir path kind track version checks deps actual required mod dep deptrack target item
 schema || bad=1
 overrides || bad=1
 while IFS=$'\t' read -r dir path kind track version checks deps;do
  if [[ ! -f "$dir/go.mod" ]];then echo "VIOLATION: $dir missing go.mod" >&2;bad=1;continue;fi
  actual=$(awk -f scripts/modules/metadata.awk "$dir/go.mod" | awk -F '\t' '$1=="module"{print $2}')
  [[ "$actual" == "$path" ]] || { echo "VIOLATION: $dir module path differs from catalog" >&2;bad=1; }
  if [[ "$deps" != - && "$deps" != stdlib ]];then
   for item in ${deps//,/ };do
    # An item without a dot in its first element names a catalog dir.
    [[ "${item%%/*}" == *.* ]] || records | awk -F '\t' -v d="$item" '$1==d{found=1} END{exit !found}' || { echo "VIOLATION: $dir deps names uncatalogued module $item" >&2;bad=1; }
   done
  fi
  if [[ "$checks" == production ]];then
   target=$(override "$dir" consumers-dir);target=${target:-examples/standalone/$dir}
   for required in minimal composed;do [[ -f "$target/$required/go.mod" ]] || { echo "VIOLATION: $dir missing $required consumer at $target" >&2;bad=1; };done
  fi
  if [[ "$track" == coordinated || "$track" == independent ]];then
   while IFS=$'\t' read -r mod dep version;do
    [[ "$mod" == require ]] || continue
    deptrack=$(records | awk -F '\t' -v p="$dep" '$2==p{print $4}')
    [[ "$deptrack" != unreleased ]] || { echo "VIOLATION: publishable $dir requires unreleased $dep" >&2;bad=1; }
   done < <(awk -f scripts/modules/metadata.awk "$dir/go.mod")
  fi
 done < <(records)
 while IFS= read -r dir;do
  [[ "$dir" != . ]] || dir=.
  records | awk -F '\t' -v d="$dir" '$1==d{found=1} END{exit !found}' || { echo "VIOLATION: uncatalogued module $dir" >&2;bad=1; }
 done < <(scan_mods .)
 actual=$(awk '
 {sub(/\/\/.*$/,"");gsub(/^[[:space:]]+|[[:space:]]+$/,"")}
 /^use[[:space:]]*\(/ {inside=1;next}
 inside && /^\)/ {inside=0;next}
 inside && NF {gsub(/"/,"");sub(/^\.\//,"");print;next}
 /^use[[:space:]]/ {sub(/^use[[:space:]]+/,"");gsub(/"/,"");sub(/^\.\//,"");print}
 ' go.work)
 if ! awk -F '\t' 'NR==FNR{if(!/^#/ && NF)expected[$1]=1;next} NF{if(seen[$0]++ || !($0 in expected)){print "VIOLATION: unexpected/duplicate go.work use " $0 > "/dev/stderr";bad=1};delete expected[$0]} END{for(d in expected){print "VIOLATION: missing go.work use " d > "/dev/stderr";bad=1};exit bad}' "$catalog" <(printf '%s\n' "$actual");then bad=1;fi
 return "$bad"
}
overrides_file="$repo/scripts/modules/overrides.tsv"
# override DIR KEY prints the value of a per-module exception, or nothing.
override(){ [[ ! -f "$overrides_file" ]] || awk -F '\t' -v d="$1" -v k="$2" '!/^#/ && $1==d && $2==k{print $3;exit}' "$overrides_file"; }
overrides(){
 [[ -f "$overrides_file" ]] || return 0
 awk -F '\t' 'NR==FNR{if(!/^#/ && NF)dirs[$1]=1;next}
 /^#/ || !NF {next}
 NF!=3 {print "overrides line " FNR ": expected three tab-separated columns" > "/dev/stderr";bad=1;next}
 !($1 in dirs) {print "overrides line " FNR ": uncatalogued dir " $1 > "/dev/stderr";bad=1}
 $2 !~ /^(consumers-dir|stress-env|env-prefix|fuzz-time|bench-time|stress-hook)$/ {print "overrides line " FNR ": unknown key " $2 > "/dev/stderr";bad=1}
 seen[$1 "\t" $2]++ {print "overrides line " FNR ": duplicate " $1 " " $2 > "/dev/stderr";bad=1}
 END{exit bad}' "$catalog" "$overrides_file"
}
lint_lists(){
 awk -F '\t' '
 NR==FNR{if(!/^#/ && NF)dirs[$1]=1;next}
 function check(line,  n,a,i,s,found){
  if(line ~ /^[[:space:]]*#/ || line ~ /modules\.sh/) return
  if(line !~ /(^|[[:space:]])(for[[:space:]].*[[:space:]]in[[:space:]]|MODULES[[:space:]]*[:?+]?=|FAMILY_MODULES[[:space:]]*[:?+]?=)/ && line !~ /dir:[[:space:]]*/)return
  n=split(line,a,/[[:space:],\[\]]+/)
  for(i=1;i<=n;i++){s=a[i];gsub(/["\047;]/,"",s);sub(/^\.\//,"",s);if(s in dirs)found=1}
  if(found){print FILENAME ":" FNR ": hard-coded module inventory: " line > "/dev/stderr";bad=1}
 }
 FNR==1{yaml=0;line=""}
 {
  if(yaml && $0 ~ /^[[:space:]]*-[[:space:]]+/)check("MODULES = " $0)
  else if($0 !~ /^[[:space:]]*$/)yaml=0
  if($0 ~ /dir:[[:space:]]*$/)yaml=1
  line=line " " $0;if(sub(/\\[[:space:]]*$/,"",line))next;check(line);line=""
 }
 END{exit bad}' "$catalog" Makefile .github/workflows/*.yml
}
command=${1:-};shift || true
case "$command" in
 dirs)
  schema;track='';kind='';checks=''
  while [[ $# -gt 0 ]];do [[ $# -ge 2 ]] || exit 2;case "$1" in --track)track=$2;;--kind)kind=$2;;--checks)checks=$2;;*)exit 2;;esac;shift 2;done
  records | awk -F '\t' -v t="$track" -v k="$kind" -v c="$checks" '(t==""||$4==t)&&(k==""||$3==k)&&(c==""||$6==c){print $1}' ;;
 override)
  [[ $# == 2 ]] || exit 2;override "$1" "$2" ;;
 get)
  [[ $# == 2 ]] || exit 2;schema;index=$(column "$2");records | awk -F '\t' -v d="$1" -v c="$index" '$1==d{print $c;found=1} END{exit !found}' ;;
 resolve)
  [[ $# == 1 ]] || exit 2;schema;records | awk -F '\t' -v n="$1" '{name=$1;gsub(/\//,"-",name);if(name==n){print $1;found=1}}END{exit !found}' ;;
 validate)validate ;;
 lint-lists)lint_lists ;;
 table)
  schema;records | awk -F '\t' 'BEGIN{print "| Directory | Module | Kind | Track | Version | Checks | Deps |";print "|---|---|---|---|---|---|---|"}{printf "| %s | %s | %s | %s | %s | %s | %s |\n",$1,$2,$3,$4,$5,$6,$7}' ;;
 matrix)
  schema;records | awk -F '\t' 'BEGIN{printf "{\"include\":["}{name=$1;gsub(/\//,"-",name);printf "%s{\"dir\":\"%s\",\"name\":\"%s\"}",sep,$1,name;sep=","}END{print "]}"}' ;;
 *)echo 'usage: modules.sh dirs|get|override|resolve|validate|table|matrix|lint-lists' >&2;exit 2 ;;
esac
