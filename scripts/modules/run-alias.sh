#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "$0")/../.." && pwd)
target=${1:?target required};stem=${target#check-};mode=consumers
case "$target" in
 check-config-engine)exec "$repo/scripts/check-module.sh" config engine;;
 bench-config-engine)exec "$repo/scripts/check-module.sh" config bench-engine;;
esac
case "$stem" in *-deps|*-quick|*-stress|*-fuzz|*-bench|*-production)
 mode=${stem##*-};stem=${stem%-*};[[ "$mode" != production ]] || mode=all ;;
esac
dir=$("$repo/scripts/modules.sh" resolve "$stem")
exec "$repo/scripts/check-module.sh" "$dir" "$mode"
