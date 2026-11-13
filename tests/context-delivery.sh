#!/usr/bin/env bash
# Live check that this checkout's knowledge reaches the main session and the subagents on the
# installed Claude Code (docs/plans/subagent-knowledge-delivery.md, AC9). It makes real model calls on
# your account: haiku for the session and, forced, for every subagent, each call capped by
# --max-budget-usd ($BUDGET, default 1). Not part of the fast tests: run it by hand after a Claude
# Code upgrade and after changing agents, skills or hooks.   Run: bash tests/context-delivery.sh
#
# It reads the transcripts Claude Code writes, not what the model says:
#   a) ecc:go-reviewer has its own prompt with ${CLAUDE_PLUGIN_ROOT} substituted, the preloaded
#      go-clean-architecture and golang-patterns text (with ${CLAUDE_PLUGIN_ROOT} and
#      ${CLAUDE_SKILL_DIR} substituted) and the line of the context-hint hook;
#   b) the built-in Explore agent, which gets no CLAUDE.md or rules, has the hook line;
#   c) after /compact the SessionStart hook runs again (source "compact") and its line follows the
#      compaction boundary in the main session.
# The throwaway project and its transcripts are removed at the end; KEEP=1 keeps them.
set -u
ROOT=$(cd -- "$(dirname -- "$0")/.." && pwd)
CONFIG=${CLAUDE_CONFIG_DIR:-$HOME/.claude}
BUDGET=${BUDGET:-1}
command -v claude >/dev/null 2>&1 || { echo "claude is not on PATH"; exit 2; }
pass=0 fail=0 cost=0
check() { # check <name> <condition-exit-code> [details]
	if [[ "$2" == 0 ]]; then pass=$((pass + 1)); echo "ok   $1"; else fail=$((fail + 1)); echo "FAIL $1${3:+: $3}"; fi
}

WORK=$(mktemp -d) && [[ -d "$WORK" ]] || { echo "mktemp -d failed"; exit 2; }
proj=$WORK/proj
TAG=$(basename "$WORK" | tr -c '[:alnum:]\n' '-') # how the path shows up in the transcript directory name
TRANSCRIPT_DIRS=()
cleanup() {
	if [[ "${KEEP:-}" == 1 ]]; then
		echo "kept: $proj and ${TRANSCRIPT_DIRS[*]:-no transcripts}"
		return
	fi
	local d
	for d in ${TRANSCRIPT_DIRS[@]+"${TRANSCRIPT_DIRS[@]}"}; do
		# only this run's own transcript directories, never anything else under the config directory
		[[ -n "$TAG" && "$d" == "$CONFIG/projects/"?* && "$(basename "$d")" == *"$TAG"* ]] && rm -rf -- "$d"
	done
	rm -rf -- "$WORK"
}
trap cleanup EXIT

mkdir -p "$proj/.claude" "$proj/backend/internal/leads/usecase"
printf '{"plugin": "ecc", "version": "live-test"}\n' >"$proj/.claude/ecc.json"
printf 'package usecase\n\nfunc Assign() {}\n' >"$proj/backend/internal/leads/usecase/assign.go"
git -C "$proj" init -q

# run <prompt> [claude args...] : one non-interactive turn in the throwaway project, this checkout
# as the only plugin (no user settings, so an installed copy of the plugin stays out); prints JSON.
run() {
	local prompt=$1
	shift
	(cd "$proj" && CLAUDE_CODE_SUBAGENT_MODEL=haiku CLAUDE_CODE_SUBAGENT_MODEL_FORCE=1 claude -p \
		--model haiku --setting-sources project,local --plugin-dir "$ROOT" --allowedTools Agent \
		--max-budget-usd "$BUDGET" --output-format json "$@" "$prompt" </dev/null 2>/dev/null)
}
jget() { python3 -I -c 'import json, sys; print(json.loads(sys.stdin.read() or "{}").get(sys.argv[1], ""))' "$1" 2>/dev/null; }
add_cost() { cost=$(python3 -I -c 'import sys; print(round(float(sys.argv[1]) + float(sys.argv[2] or 0), 4))' "$cost" "$(jget total_cost_usd <<<"$1")"); }
transcript() { find "$CONFIG/projects" -mindepth 2 -maxdepth 2 -name "$1.jsonl" 2>/dev/null | head -n 1; }
# has <transcript.jsonl> <text> : some string in the decoded transcript contains text (never "").
has() {
	[[ -n "$2" ]] || return 1
	python3 -I -c '
import json, sys
def strings(x):
    if isinstance(x, str):
        yield x
    elif isinstance(x, dict):
        for v in x.values():
            yield from strings(v)
    elif isinstance(x, list):
        for v in x:
            yield from strings(v)
for line in open(sys.argv[1], encoding="utf-8"):
    try:
        d = json.loads(line)
    except ValueError:
        continue
    if any(sys.argv[2] in s for s in strings(d)):
        sys.exit(0)
sys.exit(1)' "$1" "$2"
}
# expanded <file> <VAR> : the first line of file that uses ${VAR}, as Claude Code should deliver it.
expanded() {
	python3 -I -c '
import os, sys
path, var, root = sys.argv[1], "${" + sys.argv[2] + "}", sys.argv[3]
for line in open(path, encoding="utf-8"):
    if var in line:
        line = line.strip().replace("${CLAUDE_PLUGIN_ROOT}", root)
        print(line.replace("${CLAUDE_SKILL_DIR}", os.path.dirname(os.path.abspath(path))))
        break' "$1" "$2" "$ROOT"
}
h1() { grep -m 1 '^# ' "$ROOT/skills/$1/SKILL.md"; }
HINT="ecc harness: stack defaults and pinned versions are in $ROOT/STACK.md"

# --- a, b: a plugin agent and a built-in agent ----------------------------------------------------
out=$(run 'Use the Agent tool twice, one after the other, in the foreground, and wait for each result. First: subagent_type "ecc:go-reviewer" with the prompt "Reply with the single word READY-GR. Do not use any tool." Second: subagent_type "Explore" with the prompt "Reply with the single word READY-EX. Do not use any tool." Then reply DONE.')
add_cost "$out"
sid=$(jget session_id <<<"$out")
main=$(transcript "$sid")
[[ -n "$sid" && -n "$main" ]]; check "session with two subagents ran" $? "$(head -c 300 <<<"$out")"
[[ -n "$main" ]] && TRANSCRIPT_DIRS+=("$(dirname "$main")")
gr=$(grep -l 'READY-GR' "${main%.jsonl}"/subagents/*.jsonl 2>/dev/null | head -n 1)
ex=$(grep -l 'READY-EX' "${main%.jsonl}"/subagents/*.jsonl 2>/dev/null | head -n 1)
[[ -n "$gr" && -n "$ex" ]]; check "both subagent transcripts exist" $?
if [[ -n "$gr" ]]; then
	has "$gr" "$(expanded "$ROOT/agents/go-reviewer.md" CLAUDE_PLUGIN_ROOT)"
	check "a) go-reviewer: its prompt has the absolute STACK.md path" $?
	for s in go-clean-architecture golang-patterns; do
		has "$gr" "$(h1 "$s")"; check "a) go-reviewer: $s is preloaded" $?
	done
	f=$ROOT/skills/go-clean-architecture/SKILL.md
	has "$gr" "$(expanded "$f" CLAUDE_PLUGIN_ROOT)" && has "$gr" "$(expanded "$f" CLAUDE_SKILL_DIR)"
	check "a) go-reviewer: the preloaded skill has its paths substituted" $?
	has "$gr" "$HINT"; check "a) go-reviewer: has the hook line" $?
	# the model that served it and the size of its first request (system prompt, tools, skill
	# listing, preloaded skills, hook line): what every go-reviewer start costs in context
	info=$(python3 -I -c 'import json, sys
for l in open(sys.argv[1]):
    d = json.loads(l)
    m = d.get("message") if isinstance(d.get("message"), dict) else {}
    u = m.get("usage") if m.get("role") == "assistant" else None
    if u:
        t = u.get("input_tokens", 0) + u.get("cache_creation_input_tokens", 0) + u.get("cache_read_input_tokens", 0)
        print("model %s, first request %d tokens" % (m.get("model"), t))
        break' "$gr" 2>/dev/null)
	echo "     (go-reviewer: ${info:-model and size unknown})"
fi
if [[ -n "$ex" ]]; then
	has "$ex" "$HINT"; check "b) Explore: has the hook line" $?
fi

# --- c: the main session after /compact ---------------------------------------------------------
out=$(run 'Reply with the single word OK.')
add_cost "$out"
sid=$(jget session_id <<<"$out")
out=$(run '/compact' --resume "$sid")
add_cost "$out"
main=$(transcript "$sid")
[[ -n "$main" ]] && TRANSCRIPT_DIRS+=("$(dirname "$main")")
python3 -I -c '
import json, sys
want, boundary, ran, added = sys.argv[2], False, False, False
for line in open(sys.argv[1], encoding="utf-8"):
    try:
        d = json.loads(line)
    except ValueError:
        continue
    if d.get("type") == "system" and d.get("subtype") == "compact_boundary":
        boundary, ran, added = True, False, False
        continue
    a = d.get("attachment") if isinstance(d.get("attachment"), dict) else {}
    if boundary and a.get("hookName") == "SessionStart:compact" and want in (a.get("stdout") or ""):
        ran = True
    if boundary and a.get("type") == "hook_additional_context" and any(want in c for c in a.get("content") or []):
        added = True
sys.exit(0 if ran and added else 1)' "${main:-/dev/null}" "$HINT" 2>/dev/null
check "c) main session: the hook line is back after /compact" $?

echo "--- $pass passed, $fail failed, cost \$$cost"
[[ $fail == 0 ]]
