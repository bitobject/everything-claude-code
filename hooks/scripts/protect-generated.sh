#!/usr/bin/env bash
# PreToolUse (Edit|Write|MultiEdit): deny edits to generated files. The fix belongs in the
# source (api/openapi.yaml, queries/*.sql, the generator config), followed by regeneration.
set -u
HOOK_INPUT=$(cat)
. "$(dirname -- "$0")/lib.sh" || exit 0

file=$(json_get '.tool_input.file_path')
[[ -n "$file" ]] || exit 0

if is_generated "$file"; then
	hint="change the source and regenerate"
	case "$file" in
	*/gen/openapi/* | */lib/api/*) hint="edit api/openapi.yaml, then regenerate (Go: go generate ./... in backend/; web: the openapi-typescript script)" ;;
	*/sqlcgen/*) hint="edit the module's queries/*.sql or the migration, then run sqlc generate" ;;
	esac
	emit_deny "$file is generated (it has a generator header). Don't edit it: $hint."
fi
exit 0
