#!/usr/bin/env bash
# Stop: before Claude finishes, look at lines added since HEAD for leftover debug output
# (console.log in web code, fmt.Print*/println in non-test, non-cmd Go code). If any, ask
# Claude once to remove them or say why they stay; never loop (stop_hook_active).
set -u
HOOK_INPUT=$(cat)
. "$(dirname -- "$0")/lib.sh" || exit 0

[[ "$(json_get '.stop_hook_active')" == "true" ]] && exit 0
cwd=$(json_get '.cwd')
[[ -n "$cwd" ]] && cd -- "$cwd" 2>/dev/null
git rev-parse --git-dir >/dev/null 2>&1 || exit 0
git rev-parse --verify -q HEAD >/dev/null || exit 0

found=$(
	git diff -U0 --no-color HEAD -- '*.go' '*.ts' '*.tsx' '*.js' '*.jsx' 2>/dev/null | awk '
		/^\+\+\+ b\// { file = substr($0, 7); next }
		/^@@/ { split($3, a, ","); line = substr(a[1], 2) + 0; next }
		/^\+/ {
			text = substr($0, 2)
			skip = (file ~ /_test\.go$|(^|\/)cmd\/|\.test\.|\.spec\.|(^|\/)e2e\/|(^|\/)scripts\//)
			if (!skip && file ~ /\.go$/ && text ~ /(^|[^[:alnum:]_.])(fmt\.Print(ln|f)?|println|print)\(/) print file ":" line ": " text
			if (!skip && file ~ /\.(ts|tsx|js|jsx)$/ && text ~ /console\.(log|debug)\(/) print file ":" line ": " text
			line++
		}' | head -n 10
)

if [[ -n "$found" ]]; then
	reason="Debug output was added in this change. Remove it (Go: use the injected *slog.Logger; web: remove console.log), or tell the user why it must stay:
$found"
	printf '{"decision":"block","reason":%s}\n' "$(json_string "$reason")"
fi
exit 0
