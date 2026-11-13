#!/usr/bin/env bash
# SessionStart and SubagentStart: in a repository set up by /ecc:setup-project (it has
# .claude/ecc.json at its root), add one line saying where the plugin's STACK.md, reference app and
# skills are. Subagents never see SessionStart context, and the built-in Explore and Plan agents get
# no CLAUDE.md or rules, so for them this line is the only pointer; SessionStart also fires after
# /compact (source "compact"), so the main session keeps it. Other repositories get nothing.
# Like every hook here, it never fails the session: on any problem it prints nothing.
set -u
HOOK_INPUT=$(cat)
. "$(dirname -- "$0")/lib.sh" || exit 0

event=$(json_get '.hook_event_name')
[[ "$event" == SessionStart || "$event" == SubagentStart ]] || exit 0

# The marker counts only at the repository root, where /ecc:setup-project writes it, never above it.
# Claude Code sets CLAUDE_PROJECT_DIR to the launch directory, which can be a subdirectory such as
# web/, so take the git root from there (from cwd when the variable is missing; outside git, the
# directory itself).
base=${CLAUDE_PROJECT_DIR:-$(json_get '.cwd')}
[[ -n "$base" ]] || exit 0
project=$(git -C "$base" rev-parse --show-toplevel 2>/dev/null) || project=$base
[[ -f "$project/.claude/ecc.json" ]] || exit 0

root=${CLAUDE_PLUGIN_ROOT:-$(cd -- "$(dirname -- "$0")/../.." && pwd)}
emit_context "$event" "ecc harness: stack defaults and pinned versions are in $root/STACK.md; an accepted ADR in docs/adr/ wins over it. Compiling reference app: $root/skills/go-clean-architecture/reference/. This project's conventions live in the ecc:* skills: before you start, load the ones that match your task with the Skill tool, unless they are already in your context."
