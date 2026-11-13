#!/usr/bin/env bash
# Fails when a pinned tool has no version for this platform, is missing from the bin directory, or
# isn't the pinned version, and when the Go that backend/ runs isn't the one go.mod pins (first
# step of make check).
# Usage: check-tools.sh <lock file> <bin dir>
set -uo pipefail
lock=${1:?usage: check-tools.sh <lock file> <bin dir>}
bin=${2:?usage: check-tools.sh <lock file> <bin dir>}
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=scripts/lib.sh
. "$here/lib.sh"
platform=$(tool_platform)
backend="$here/../backend"

status=0

# Go itself. The go line in go.mod is only a minimum, so a newer local Go would run silently, and
# the pinned linters are built for the pinned Go. "go 1.27" accepts any 1.27.x; "go 1.27.4" (or a
# toolchain line) accepts exactly that release. Asked from backend/, so GOTOOLCHAIN applies.
go_pinned=$(awk '$1 == "toolchain" { sub(/^go/, "", $2); print $2; exit }' "$backend/go.mod")
[ -n "$go_pinned" ] || go_pinned=$(awk '$1 == "go" { print $2; exit }' "$backend/go.mod")
go_installed=$(cd "$backend" && go env GOVERSION 2>/dev/null)
go_installed=${go_installed#go}
go_compared=$go_installed
case $go_pinned in
  *.*.*) ;;                                               # exact release
  *) go_compared=$(echo "$go_installed" | cut -d. -f1,2) ;; # language version only
esac
if [ "$go_compared" != "$go_pinned" ]; then
  echo "go is ${go_installed:-missing (go env GOVERSION failed in backend/)}, backend/go.mod pins $go_pinned: install Go $go_pinned" >&2
  status=1
fi

for name in $(lock_tools "$lock"); do
  read -r version _ <<<"$(lock_line "$lock" "$name" "$platform")"
  if [ -z "$version" ]; then
    echo "$name has no pinned version for $platform in $lock: add a line with its version and SHA-256" >&2
    status=1
  elif [ ! -x "$bin/$name" ]; then
    echo "$name is not installed, pinned $version: run make tools" >&2
    status=1
  else
    installed=$(tool_version "$bin/$name")
    if [ "$installed" != "$version" ]; then
      echo "$name is ${installed:-an unknown version}, pinned $version: run make tools" >&2
      status=1
    fi
  fi
done
exit "$status"
