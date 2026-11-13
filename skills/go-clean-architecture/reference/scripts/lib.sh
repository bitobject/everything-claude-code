# Shared helpers for the tool scripts. Source it: . "$(dirname "$0")/lib.sh"

# tool_platform prints this machine as os-arch, the way tools.lock names platforms (linux-amd64).
tool_platform() {
  local arch
  case $(uname -m) in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) arch=$(uname -m) ;;
  esac
  echo "$(uname -s | tr '[:upper:]' '[:lower:]')-$arch"
}

# tool_version <binary> prints the first x.y.z in what the tool reports as its version.
tool_version() {
  local how=--version
  case ${1##*/} in
    sqlc | go-arch-lint | vacuum) how=version ;; # a subcommand, not a flag
  esac
  "$1" "$how" 2>&1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -n 1
}

# lock_tools <lock file> prints each tool named in the lock file once.
lock_tools() {
  awk '!/^#/ && NF { print $1 }' "$1" | sort -u
}

# lock_line <lock file> <name> <platform> prints "version sha256" for that tool on that platform.
lock_line() {
  awk -v n="$2" -v p="$3" '!/^#/ && $1 == n && $3 == p { print $2, $4; exit }' "$1"
}
