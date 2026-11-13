#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
track=coordinated
if [[ $# -gt 0 ]];then [[ $# == 2 && "$1" == --track ]] || exit 2;track=$2;fi
case "$track" in coordinated|independent);;*)echo 'only approved coordinated/independent tracks can release' >&2;exit 2;;esac
"$repo/scripts/modules.sh" validate
while IFS= read -r dir;do
 version=$("$repo/scripts/modules.sh" get "$dir" version)
 if [[ "$dir" == . ]];then printf '%s\n' "$version";else printf '%s/%s\n' "$dir" "$version";fi
done < <("$repo/scripts/modules.sh" dirs --track "$track")
