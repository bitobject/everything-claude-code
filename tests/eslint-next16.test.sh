#!/usr/bin/env bash
# The Next.js 16 rules of the ESLint config in skills/coding-standards/SKILL.md (the block projects
# copy to web/eslint.config.mjs), run on tests/fixtures/eslint-next16 with the versions STACK.md §4
# pins. No model calls. It needs Node, pnpm and, on the first run, the npm registry (pnpm's store
# keeps the packages). Run it after changing that config or those versions.
#   Run: bash tests/eslint-next16.test.sh        KEEP=1 keeps the throwaway app.
#
# A fixture line that must be reported carries "expect: <part of the message>" in a comment (several
# parts: "a | b"). Every no-restricted-imports and no-restricted-syntax report must be expected on its
# line, and every expectation must be reported there. Reports from other rules are not checked.
set -u
ROOT=$(cd -- "$(dirname -- "$0")/.." && pwd)
for tool in node pnpm python3; do
	command -v "$tool" >/dev/null 2>&1 || { echo "$tool is not on PATH"; exit 2; }
done
WORK=$(mktemp -d) && [[ -d "$WORK" ]] || { echo "mktemp -d failed"; exit 2; }
if [[ "${KEEP:-}" == 1 ]]; then trap 'echo "kept: $WORK/web"' EXIT; else trap 'rm -rf -- "$WORK"' EXIT; fi
web=$WORK/web
mkdir -p "$web" && cp -R "$ROOT/tests/fixtures/eslint-next16/." "$web/"

# package.json and .tool-versions from STACK.md §4, eslint.config.mjs from the skill, a tsconfig like
# the one create-next-app writes.
python3 -I - "$ROOT" "$web" <<'PY' || exit 2
import json, pathlib, re, sys

root, web = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])
stack = (root / "STACK.md").read_text(encoding="utf-8")
section = re.search(r"^## 4\. .*?(?=^## )", stack, re.S | re.M)
if not section:
    sys.exit("STACK.md has no '## 4.' section")


def version(label):
    found = re.search(r"(?<![\w@/.-])" + re.escape(label) + r" (\d+\.\d+\.\d+)\b", section.group(0))
    if not found:
        sys.exit(f"STACK.md §4 has no version for {label}")
    return found.group(1)


types_react = version("@types/react and @types/react-dom")
dev = {
    "next": version("Next.js"),
    "react": version("React"),
    "react-dom": version("React"),
    "typescript": version("TypeScript"),
    "@types/react": types_react,
    "@types/react-dom": types_react,
    "@types/node": version("@types/node"),
    "eslint": version("ESLint"),
    "eslint-config-next": version("eslint-config-next"),
    "typescript-eslint": version("typescript-eslint"),
    "eslint-plugin-boundaries": version("eslint-plugin-boundaries"),
    "eslint-plugin-jsx-a11y-x": version("eslint-plugin-jsx-a11y-x"),
    "eslint-config-prettier": version("eslint-config-prettier"),
}
package = {"name": "eslint-next16-fixture", "private": True, "type": "module", "devDependencies": dev}
(web / "package.json").write_text(json.dumps(package, indent=2) + "\n", encoding="utf-8")
(web / ".tool-versions").write_text(f"nodejs {version('Node.js')}\npnpm {version('pnpm')}\n", encoding="utf-8")

skill = (root / "skills/coding-standards/SKILL.md").read_text(encoding="utf-8")
block = re.search(r"^```js\n(// web/eslint\.config\.mjs\n.*?)^```$", skill, re.S | re.M)
if not block:
    sys.exit("skills/coding-standards/SKILL.md has no web/eslint.config.mjs block")
(web / "eslint.config.mjs").write_text(block.group(1), encoding="utf-8")

tsconfig = {
    "compilerOptions": {
        "target": "ES2022",
        "lib": ["dom", "dom.iterable", "esnext"],
        "skipLibCheck": True,
        "strict": True,
        "noEmit": True,
        "module": "esnext",
        "moduleResolution": "bundler",
        "isolatedModules": True,
        "jsx": "react-jsx",
        "plugins": [{"name": "next"}],
        "paths": {"@/*": ["./src/*"]},
    },
    "include": ["**/*.ts", "**/*.tsx"],
    "exclude": ["node_modules"],
}
(web / "tsconfig.json").write_text(json.dumps(tsconfig, indent=2) + "\n", encoding="utf-8")
print("versions from STACK.md: " + ", ".join(f"{k} {v}" for k, v in dev.items()))
PY

if ! (cd "$web" && pnpm install --no-frozen-lockfile --reporter=append-only >"$WORK/install.log" 2>&1); then
	tail -n 20 "$WORK/install.log"
	echo "pnpm install failed"
	exit 2
fi

# ESLint exits 1 when it reports problems (expected here) and 2 when it cannot run.
(cd "$web" && pnpm exec eslint --format json . >"$WORK/report.json" 2>"$WORK/eslint.err")
status=$?
if ((status > 1)); then
	cat "$WORK/eslint.err"
	echo "eslint could not run (exit $status)"
	exit 1
fi

python3 -I - "$web" "$WORK/report.json" <<'PY'
import json, os, pathlib, re, sys

web = os.path.realpath(sys.argv[1])
report = json.load(open(sys.argv[2], encoding="utf-8"))
RESTRICTED = {"no-restricted-imports", "no-restricted-syntax"}
EXPECT = re.compile(r"expect: (.+?)\s*(?:\*/\})?\s*$")

expected, fixtures = {}, set()
for path in sorted(pathlib.Path(web, "src").rglob("*")):
    if path.suffix not in {".ts", ".tsx"}:
        continue
    rel = path.relative_to(web).as_posix()
    fixtures.add(rel)
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        found = EXPECT.search(line)
        if found:
            expected[(rel, number)] = [part.strip() for part in found.group(1).split(" | ")]

actual, linted, broken, other = {}, set(), [], 0
for entry in report:
    rel = pathlib.Path(os.path.relpath(os.path.realpath(entry["filePath"]), web)).as_posix()
    linted.add(rel)
    for msg in entry["messages"]:
        if msg.get("fatal") or msg.get("ruleId") is None:
            broken.append(f"{rel}:{msg.get('line')}: {msg['message']}")
        elif msg["ruleId"] in RESTRICTED:
            actual.setdefault((rel, msg["line"]), []).append(msg["message"])
        else:
            other += 1

passed = failed = 0


def check(ok, text):
    global passed, failed
    if ok:
        passed += 1
        print(f"ok   {text}")
    else:
        failed += 1
        print(f"FAIL {text}")


for rel in sorted(fixtures - linted):
    check(False, f"{rel} was not linted")
for problem in broken:
    check(False, f"ESLint could not lint it: {problem}")
for (rel, line), parts in sorted(expected.items()):
    messages = actual.get((rel, line), [])
    for part in parts:
        check(any(part in m for m in messages), f"{rel}:{line} reports “{part}”" + ("" if messages else " (nothing reported)"))
for (rel, line), messages in sorted(actual.items()):
    for m in messages:
        if not any(part in m for part in expected.get((rel, line), [])):
            check(False, f"{rel}:{line} reports what no fixture expects: {m}")

print(f"--- {passed} passed, {failed} failed ({other} reports from other rules, not checked here)")
sys.exit(1 if failed else 0)
PY
