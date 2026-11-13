#!/usr/bin/env bash
# Installs the pinned tools from the lock file into the bin directory (make tools). Each download's
# SHA-256 is checked before it is unpacked, in a fresh directory of its own. Tools already at the
# pinned version are skipped. Ends with check-tools.sh, which fails for anything still missing.
# Usage: install-tools.sh <lock file> <bin dir>
set -euo pipefail
lock=${1:?usage: install-tools.sh <lock file> <bin dir>}
bin=${2:?usage: install-tools.sh <lock file> <bin dir>}
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=scripts/lib.sh
. "$here/lib.sh"
platform=$(tool_platform)

# url_for <name> <version> <platform>: where the release is downloaded from: a .tar.gz archive
# holding the binary, or the binary itself. Each project names its assets its own way.
url_for() {
  local os=${3%-*} arch=${3#*-}
  case $1 in
    golangci-lint) echo "https://github.com/golangci/golangci-lint/releases/download/v$2/golangci-lint-$2-$3.tar.gz" ;;
    sqlc) echo "https://github.com/sqlc-dev/sqlc/releases/download/v$2/sqlc_$2_${os}_$arch.tar.gz" ;;
    go-arch-lint) echo "https://github.com/fe3dback/go-arch-lint/releases/download/v$2/go-arch-lint_$2_${os}_$arch.tar.gz" ;;
    goose)
      [ "$arch" = amd64 ] && arch=x86_64
      echo "https://github.com/pressly/goose/releases/download/v$2/goose_${os}_$arch"
      ;;
    squawk)
      [ "$arch" = amd64 ] && arch=x64
      echo "https://github.com/sbdchd/squawk/releases/download/v$2/squawk-$os-$arch"
      ;;
    vacuum)
      [ "$arch" = amd64 ] && arch=x86_64
      echo "https://github.com/daveshanley/vacuum/releases/download/v$2/vacuum_$2_${os}_$arch.tar.gz"
      ;;
    oasdiff) echo "https://github.com/oasdiff/oasdiff/releases/download/v$2/oasdiff_$2_${os}_$arch.tar.gz" ;;
    *)
      echo "install-tools.sh: no download rule for $1" >&2
      return 1
      ;;
  esac
}

mkdir -p "$bin"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

for name in $(lock_tools "$lock"); do
  read -r version sum <<<"$(lock_line "$lock" "$name" "$platform")"
  [ -n "$version" ] || continue # no pin for this platform: check-tools.sh below reports it
  if [ -x "$bin/$name" ] && [ "$(tool_version "$bin/$name")" = "$version" ]; then
    echo "$name $version: already installed"
    continue
  fi
  dir="$tmp/$name"
  mkdir -p "$dir"
  url=$(url_for "$name" "$version" "$platform")
  curl -fsSL --retry 3 -o "$dir/download" "$url"
  if ! echo "$sum  $dir/download" | sha256sum -c --quiet -; then
    echo "$name: checksum mismatch, not installed" >&2
    exit 1
  fi
  case $url in
    *.tar.gz)
      tar -xzf "$dir/download" -C "$dir"
      found=$(find "$dir" -type f -name "$name" -perm -u+x | head -n 1)
      if [ -z "$found" ]; then
        echo "$name: no $name binary in the archive" >&2
        exit 1
      fi
      ;;
    *) found="$dir/download" ;; # the release is the binary itself
  esac
  install -m 0755 "$found" "$bin/$name"
  echo "$name $version: installed"
done

bash "$here/check-tools.sh" "$lock" "$bin"
