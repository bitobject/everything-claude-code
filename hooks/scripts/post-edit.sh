#!/usr/bin/env bash
# PostToolUse (Edit|Write|MultiEdit): format the edited file and report problems back to Claude.
#   .go          goimports -local <module> (or gofmt) -w; syntax errors; `go vet` of the package;
#                debug prints. -local groups the module's own imports last, as golangci-lint's
#                goimports local-prefixes expects (set it to the module path in .golangci.yml).
#   .ts/.tsx/... local prettier --write; tsc errors in this file; console.log in non-test code
# Only locally installed tools run; nothing is downloaded. Disable parts with
# ECC_GO_VET=0 or ECC_TSC=0 (e.g. during a large refactor).
set -u
HOOK_INPUT=$(cat)
. "$(dirname -- "$0")/lib.sh" || exit 0

file=$(json_get '.tool_input.file_path')
[[ -n "$file" && -f "$file" ]] || exit 0
is_generated "$file" && exit 0

dir=$(dirname -- "$file")
notes=()

case "$file" in
*.go)
	root=$(find_up "$dir" go.mod) || root=""
	if command -v goimports >/dev/null 2>&1; then
		local_flag=()
		if [[ -n "$root" ]]; then
			mod=$(awk '$1 == "module" { print $2; exit }' "$root/go.mod" 2>/dev/null)
			[[ -n "$mod" ]] && local_flag=(-local "$mod")
		fi
		out=$(goimports "${local_flag[@]}" -w -- "$file" 2>&1) || notes+=("goimports failed (syntax error?):"$'\n'"$out")
	elif command -v gofmt >/dev/null 2>&1; then
		out=$(gofmt -w -- "$file" 2>&1) || notes+=("gofmt failed (syntax error?):"$'\n'"$out")
	fi

	if [[ "${ECC_GO_VET:-1}" != 0 && ${#notes[@]} -eq 0 ]] && command -v go >/dev/null 2>&1; then
		if [[ -n "$root" ]]; then
			rel=${dir#"$root"}
			out=$(cd -- "$root" && timeout 90 go vet "./${rel#/}" 2>&1) ||
				notes+=("go vet ./${rel#/} reports (the package may simply be mid-edit; fix before finishing):"$'\n'"$(grep -v '^#' <<<"$out" | head -n 20)")
		fi
	fi

	case "$file" in
	*_test.go | */cmd/*) ;;
	*)
		hits=$(grep -nE '(^|[^[:alnum:]_.])(fmt\.Print(ln|f)?|println|print)\(' -- "$file" | head -n 5)
		[[ -n "$hits" ]] && notes+=("Debug output in $file (use the injected *slog.Logger, or remove):"$'\n'"$hits")
		;;
	esac
	;;

*.ts | *.tsx | *.js | *.jsx | *.mjs | *.cjs | *.css | *.scss | *.json | *.md | *.mdx | *.yaml | *.yml)
	# Formatting only inside a JS project (a package.json above the file) with prettier installed.
	if find_up "$dir" package.json >/dev/null && prettier=$(node_bin "$dir" prettier); then
		"$prettier" --write --log-level warn -- "$file" >/dev/null 2>&1 || true
	fi

	case "$file" in
	*.ts | *.tsx)
		if [[ "${ECC_TSC:-1}" != 0 ]] && proj=$(find_up "$dir" tsconfig.json) && tsc=$(node_bin "$proj" tsc); then
			out=$(cd -- "$proj" && timeout 90 "$tsc" --noEmit --pretty false 2>&1)
			rel=${file#"$proj"/}
			errs=$(grep -F -- "$rel" <<<"$out" | head -n 10)
			[[ -n "$errs" ]] && notes+=("TypeScript errors in $rel:"$'\n'"$errs")
		fi
		;;
	esac

	case "$file" in
	*.test.* | *.spec.* | */e2e/* | */tests/* | */__tests__/* | */scripts/*) ;;
	*.ts | *.tsx | *.js | *.jsx)
		hits=$(grep -nE 'console\.(log|debug)\(' -- "$file" | head -n 5)
		[[ -n "$hits" ]] && notes+=("console.log in $file (remove before finishing):"$'\n'"$hits")
		;;
	esac
	;;
esac

if [[ ${#notes[@]} -gt 0 ]]; then
	msg=$(printf '%s\n\n' "${notes[@]}")
	emit_context PostToolUse "$msg"
fi
exit 0
