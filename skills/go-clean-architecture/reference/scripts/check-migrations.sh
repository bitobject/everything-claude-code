#!/usr/bin/env bash
# Checks the goose migration versions (make migrations-check): no version twice (goose validate
# doesn't catch that), and, in CI (CI=true), none still named with a timestamp.
# A new migration is created with a timestamp (make migration name=...), so parallel branches
# never pick the same number; `make migrations-fix` renumbers it right before merging. If main got
# the same number meanwhile, give yours the next one.
# Usage: check-migrations.sh <migrations dir>
set -euo pipefail
dir=${1:?usage: check-migrations.sh <migrations dir>}

versions=$(find "$dir" -maxdepth 1 -name '*.sql' -printf '%f\n' | cut -d_ -f1 | sed 's/^0*//' | sort -n)
dups=$(echo "$versions" | uniq -d)
if [ -n "$dups" ]; then
  echo "migrations: version used twice: $(echo "$dups" | tr '\n' ' ')- renumber yours" >&2
  exit 1
fi

stamped=$(echo "$versions" | awk 'length($0) >= 12')
if [ -n "$stamped" ]; then
  if [ "${CI:-}" = true ]; then
    echo "migrations: timestamp-named files left: run make migrations-fix before merging" >&2
    exit 1
  fi
  echo "migrations: run make migrations-fix before merging (timestamp-named: $(echo "$stamped" | tr '\n' ' '))"
fi
