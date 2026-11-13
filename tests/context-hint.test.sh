#!/usr/bin/env bash
# Tests for hooks/scripts/context-hint.sh (SessionStart and SubagentStart). Run: bash tests/context-hint.test.sh
# Each case feeds the hook the JSON Claude Code would send and checks its output. No model calls.
set -u
ROOT=$(cd -- "$(dirname -- "$0")/.." && pwd)
H="$ROOT/hooks/scripts"
TMP=$(mktemp -d)
trap 'rm -rf -- "$TMP"' EXIT
pass=0 fail=0

check() { # check <name> <condition-exit-code> [details]
	if [[ "$2" == 0 ]]; then pass=$((pass + 1)); echo "ok   $1"; else fail=$((fail + 1)); echo "FAIL $1${3:+: $3}"; fi
}
hint() { # hint <CLAUDE_PROJECT_DIR, may be empty> <json> -> stdout of the hook
	printf '%s' "$2" | CLAUDE_PLUGIN_ROOT="$ROOT" CLAUDE_PROJECT_DIR="$1" "$H/context-hint.sh"
}
field() { # field <hook output> <key of hookSpecificOutput>
	python3 -I -c 'import json, sys; print(json.loads(sys.argv[1])["hookSpecificOutput"][sys.argv[2]])' "$1" "$2" 2>/dev/null
}
sub_json() { printf '{"hook_event_name":"SubagentStart","agent_type":"%s","agent_id":"a1","cwd":"%s"}' "$1" "$2"; }
ss_json() { printf '{"hook_event_name":"SessionStart","source":"%s","cwd":"%s"}' "$1" "$2"; }

proj="$TMP/proj" bare="$TMP/bare" parent="$TMP/parent"
mkdir -p "$proj/.claude" "$proj/backend/internal/x" "$bare" "$parent/.claude" "$parent/child"
printf '{"plugin": "ecc", "version": "test"}\n' >"$proj/.claude/ecc.json"
cp "$proj/.claude/ecc.json" "$parent/.claude/ecc.json"
git -C "$proj" init -q
git -C "$parent/child" init -q # a repository nested in a directory that has a marker

# --- a project set up by /ecc:setup-project gets the line --------------------------------------
for at in ecc:go-reviewer Explore Plan general-purpose; do
	out=$(hint "$proj" "$(sub_json "$at" "$proj")")
	ctx=$(field "$out" additionalContext)
	[[ "$(field "$out" hookEventName)" == SubagentStart && "$ctx" == *"$ROOT/STACK.md"* &&
		"$ctx" == *"$ROOT/skills/go-clean-architecture/reference/"* && "$ctx" == *"ecc:"*"Skill tool"* ]]
	check "SubagentStart ($at) gets STACK.md, the reference and the skills line" $? "$out"
done
for src in startup resume clear compact fork; do
	out=$(hint "$proj" "$(ss_json "$src" "$proj")")
	[[ "$(field "$out" hookEventName)" == SessionStart && "$(field "$out" additionalContext)" == *"$ROOT/STACK.md"* ]]
	check "SessionStart ($src)" $? "$out"
done
out=$(hint "" "$(sub_json Explore "$proj/backend/internal/x")")
[[ "$(field "$out" additionalContext)" == *"$ROOT/STACK.md"* ]]
check "without CLAUDE_PROJECT_DIR, finds the marker at the git root of cwd" $? "$out"
# Claude Code sets CLAUDE_PROJECT_DIR to the launch directory, e.g. web/ after `cd web && claude`.
out=$(hint "$proj/backend/internal/x" "$(ss_json startup "$proj/backend/internal/x")")
[[ "$(field "$out" additionalContext)" == *"$ROOT/STACK.md"* ]]
check "launched from a subdirectory, finds the marker at the repository root" $? "$out"
out=$(printf '%s' "$(sub_json Explore "$proj")" | env -u CLAUDE_PLUGIN_ROOT CLAUDE_PROJECT_DIR="$proj" "$H/context-hint.sh")
[[ "$(field "$out" additionalContext)" == *"$ROOT/STACK.md"* ]]
check "without CLAUDE_PLUGIN_ROOT, takes the plugin root from its own location" $? "$out"
out=$(hint "$proj" "$(sub_json Explore "$proj")"); ctx=$(field "$out" additionalContext)
((${#ctx} > 0 && ${#ctx} < 1000)); check "the line is shorter than 1000 characters (${#ctx})" $?

# --- anywhere else it stays silent and never fails --------------------------------------------
out=$(hint "$bare" "$(sub_json Explore "$bare")"); rc=$?
[[ $rc == 0 && -z "$out" ]]; check "silent in a project without the marker" $? "$out"
out=$(hint "$parent/child" "$(sub_json Explore "$parent/child")"); rc=$?
[[ $rc == 0 && -z "$out" ]]; check "ignores a marker above the repository root" $? "$out"
out=$(hint "" "$(sub_json Explore "$parent/child")"); rc=$?
[[ $rc == 0 && -z "$out" ]]; check "ignores a marker above the repository root, without CLAUDE_PROJECT_DIR" $? "$out"
out=$(hint "$proj" 'not json'); rc=$?
[[ $rc == 0 && -z "$out" ]]; check "silent on broken input" $? "$out"
out=$(hint "$proj" '{"hook_event_name":"Stop"}'); rc=$?
[[ $rc == 0 && -z "$out" ]]; check "silent for other events" $? "$out"

# --- hooks.json registers it for every source and every agent ---------------------------------
[[ -x "$H/context-hint.sh" ]]; check "context-hint.sh is executable" $?
python3 -I -c '
import json, sys
h = json.load(open(sys.argv[1]))["hooks"]
def ok(event):
    groups = h.get(event, [])
    runs = any("context-hint.sh" in x.get("command", "") for g in groups for x in g.get("hooks", []))
    return runs and all("matcher" not in g for g in groups)
sys.exit(0 if ok("SessionStart") and ok("SubagentStart") else 1)' "$ROOT/hooks/hooks.json"
check "hooks.json runs it on SessionStart and SubagentStart, without a matcher" $?

echo "--- $pass passed, $fail failed"
[[ $fail == 0 ]]
