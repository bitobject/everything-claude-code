#!/usr/bin/env bash
# Static checks of the plugin's agents and the files they point to. Run: bash tests/agents.test.sh
# No model calls. The role table is the one in docs/plans/subagent-knowledge-delivery.md: tools are
# inherited (no `tools:`), each role takes away what it must not use (`disallowedTools`), and the
# one or two skills an agent always needs are preloaded (`skills:`); the rest it loads with Skill.
set -u
ROOT=$(cd -- "$(dirname -- "$0")/.." && pwd)
pass=0 fail=0

check() { # check <name> <condition-exit-code> [details]
	if [[ "$2" == 0 ]]; then pass=$((pass + 1)); echo "ok   $1"; else fail=$((fail + 1)); echo "FAIL $1${3:+: $3}"; fi
}

# fm <file> <key> : a frontmatter value. Lists (YAML or comma-separated) come out sorted and
# comma-separated; an empty value prints <empty>; a missing key prints nothing.
fm() {
	python3 -I -c '
import sys
lines = open(sys.argv[1], encoding="utf-8").read().split("\n")
if not lines or lines[0] != "---" or "---" not in lines[1:]:
    sys.exit(0)
front, key = lines[1:lines.index("---", 1)], sys.argv[2]
for i, line in enumerate(front):
    if line.split(":", 1)[0] != key or ":" not in line:
        continue
    rest, vals = line.split(":", 1)[1].strip(), []
    if rest:
        vals = [v.strip() for v in rest.strip("[]").split(",") if v.strip()]
    else:
        for nxt in front[i + 1:]:
            if nxt.lstrip().startswith("- "):
                vals.append(nxt.split("- ", 1)[1].strip())
            elif nxt.strip():
                break
    print(",".join(sorted(v.strip("\"'"'"'") for v in vals)) if vals else "<empty>")
    break
' "$1" "$2"
}

REVIEW=Agent,Edit,NotebookEdit,Write
expect() { # expect <agent> -> "<disallowedTools>|<skills>", both sorted and comma-separated
	case "$1" in
	code-reviewer) echo "$REVIEW|openapi-contract-first" ;;
	go-reviewer) echo "$REVIEW|go-clean-architecture,golang-patterns" ;;
	database-reviewer) echo "$REVIEW|postgres-patterns" ;;
	typescript-reviewer) echo "$REVIEW|nextjs-patterns" ;;
	security-reviewer) echo "$REVIEW|security-review" ;;
	seo-specialist) echo "$REVIEW|seo" ;;
	architect | planner) echo "Agent|go-clean-architecture" ;;
	tdd-guide) echo "Agent|go-clean-architecture,tdd-workflow" ;;
	go-build-resolver | build-error-resolver | e2e-runner | doc-updater | refactor-cleaner) echo "Agent|" ;;
	*) echo "unknown" ;;
	esac
}

# --- frontmatter: inherited tools, role restrictions, preloaded skills (AC5, AC6) ---------------
count=0
for f in "$ROOT"/agents/*.md; do
	a=$(basename "$f" .md) count=$((count + 1))
	want=$(expect "$a")
	[[ "$want" != unknown ]]; check "$a: is in the role table" $?
	[[ "$(fm "$f" name)" == "$a" ]]; check "$a: name matches the file name" $? "$(fm "$f" name)"
	[[ -z "$(fm "$f" tools)" ]]; check "$a: no tools: allowlist (tools are inherited)" $? "$(fm "$f" tools)"
	got=$(fm "$f" disallowedTools)
	[[ "$got" == "${want%%|*}" ]]; check "$a: disallowedTools = ${want%%|*}" $? "$got"
	got=$(fm "$f" skills)
	[[ "$got" == "${want#*|}" ]]; check "$a: skills = ${want#*|}" $? "$got"
	IFS=, read -r -a preload <<<"${want#*|}"
	((${#preload[@]} <= 32)); check "$a: at most 32 preloaded skills" $?
	for s in ${preload[@]+"${preload[@]}"}; do
		[[ -f "$ROOT/skills/$s/SKILL.md" && "$(fm "$ROOT/skills/$s/SKILL.md" disable-model-invocation)" != true ]]
		check "$a: preloaded skill $s exists and can be preloaded" $?
	done
done
((count == 14)); check "14 agents" $? "$count"

# --- references: absolute STACK.md, skills that exist (AC7) --------------------------------------
# bare_stack <files...> : file:line of each STACK.md not written as ${CLAUDE_PLUGIN_ROOT}/STACK.md.
bare_stack() {
	python3 -I -c '
import os, re, sys
for path in sys.argv[1:]:
    where = os.path.join(os.path.basename(os.path.dirname(path)), os.path.basename(path))
    for n, line in enumerate(open(path, encoding="utf-8"), 1):
        if re.search(r"(?<!\$\{CLAUDE_PLUGIN_ROOT\}/)STACK\.md", line):
            print("%s:%d" % (where, n))
' "$@"
}
# unknown_ecc <files...> : each ecc:<name> that is neither a skill, a command nor an agent.
unknown_ecc() {
	local known
	known=$(
		cd "$ROOT" && {
			ls skills
			ls commands agents | sed -n 's/\.md$//p'
		}
	)
	grep -o -h -E 'ecc:[a-z0-9-]+' "$@" | sort -u | while read -r ref; do
		grep -qx -- "${ref#ecc:}" <<<"$known" || echo "$ref"
	done
}
AGENTS=("$ROOT"/agents/*.md)
out=$(bare_stack "${AGENTS[@]}") && [[ -z "$out" ]]
check "agents: STACK.md always as \${CLAUDE_PLUGIN_ROOT}/STACK.md" $? "$(tr '\n' ' ' <<<"$out")"
out=$(unknown_ecc "${AGENTS[@]}")
[[ -z "$out" ]]; check "agents: every ecc:<name> exists" $? "$(tr '\n' ' ' <<<"$out")"
for f in "${AGENTS[@]}"; do
	a=$(basename "$f" .md)
	# only names under skills/ count: /ecc:build-fix in a description is a command, not a skill
	named=$(grep -o -E 'ecc:[a-z0-9-]+' "$f" | sed 's/^ecc://' | while read -r n; do
		[[ -f "$ROOT/skills/$n/SKILL.md" ]] && echo "$n"
	done)
	[[ -n "$named" ]] && grep -q 'Skill tool' "$f"
	check "$a: names its skills as ecc:<skill> and says to load them with the Skill tool" $?
done

SKILLMD=("$ROOT"/skills/*/SKILL.md)
COMMANDS=("$ROOT"/commands/*.md)
out=$(bare_stack "${SKILLMD[@]}" "${COMMANDS[@]}") && [[ -z "$out" ]]
check "skills and commands: STACK.md always as \${CLAUDE_PLUGIN_ROOT}/STACK.md" $? "$(tr '\n' ' ' <<<"$out")"
out=$(unknown_ecc "${SKILLMD[@]}" "${COMMANDS[@]}") && [[ -z "$out" ]]
check "skills and commands: every ecc:<name> exists" $? "$(tr '\n' ' ' <<<"$out")"
# Supporting files are opened with Read, which substitutes nothing: there the variables would stay
# literal, so only SKILL.md, commands and agents may use them.
out=$(grep -l -E '\$\{CLAUDE_(PLUGIN_ROOT|SKILL_DIR)\}' -r "$ROOT/skills" --include='*.md' | grep -v '/SKILL\.md$')
[[ -z "$out" ]]; check "no \${CLAUDE_*} variables in skill supporting files" $? "$out"
out=$(grep -l '\.\./go-clean-architecture/reference/' "${SKILLMD[@]}" | grep -v '/setup-project/')
[[ -z "$out" ]]; check "skills reach the reference through \${CLAUDE_PLUGIN_ROOT} (setup-project keeps base-relative paths)" $? "$out"
grep -q '${CLAUDE_SKILL_DIR}/reference/' "$ROOT/skills/go-clean-architecture/SKILL.md"
check "go-clean-architecture names its own reference through \${CLAUDE_SKILL_DIR}" $?
# A preloaded or invoked skill resolves short paths against its own directory, and only
# go-clean-architecture has a reference/ there.
out=$(grep -n '`reference/' "${SKILLMD[@]}" | grep -v '/go-clean-architecture/SKILL\.md:')
[[ -z "$out" ]]; check "only go-clean-architecture writes short reference/ paths" $? "$(tr '\n' ' ' <<<"$out")"
out=$(grep -rlE '\$\{CLAUDE_(PLUGIN_ROOT|SKILL_DIR)\}' "$ROOT/rules" "$ROOT/examples")
[[ -z "$out" ]]; check "rules/ and examples/, which are copied into projects, use no \${CLAUDE_*} variables" $? "$out"

# --- setup-project writes the marker the hook looks for (AC8) ------------------------------------
# step <file> <n> : the text of numbered step n, up to the next numbered step, on one line.
step() {
	python3 -I -c '
import re, sys
text = open(sys.argv[1], encoding="utf-8").read()
m = re.search(r"^%s\. (.*?)(?=^\d+\. |\Z)" % re.escape(sys.argv[2]), text, re.S | re.M)
print(" ".join(m.group(1).split()) if m else "")' "$1" "$2"
}
s7=$(step "$ROOT/skills/setup-project/SKILL.md" 7)
[[ "$s7" == 'Marker: write `<repo root>/.claude/ecc.json`'* && "$s7" == *plugin.json* && "$s7" == *"whatever the target"* ]]
check "setup-project step 7 writes .claude/ecc.json at the repository root, for every target" $? "$s7"
s8=$(step "$ROOT/skills/setup-project/SKILL.md" 8)
[[ "$s8" == Report* && "$s8" == *marker* ]]
check "setup-project step 8 reports the marker" $? "$s8"

echo "--- $pass passed, $fail failed"
[[ $fail == 0 ]]
