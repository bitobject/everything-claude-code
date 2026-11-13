<!-- Template for web/CLAUDE.md (the Next.js app). Copy it to web/CLAUDE.md, next to the AGENTS.md that
     create-next-app writes and `next dev` keeps current (between its BEGIN/END markers; don't edit that block).
     Why both files: Claude Code reads AGENTS.md on its own only when no CLAUDE.md exists in the working directory
     or above it, and this repository has one at its root. This file loads when Claude works in web/, and the
     import below pulls in web/AGENTS.md, which points to the docs of the installed Next.js version.
     Block comments like this one are not loaded into context. -->
@AGENTS.md

# web/

- Next.js API questions: the docs bundled with the installed version, `node_modules/next/dist/docs/`.
  Our decisions: the ecc skills `nextjs-patterns`, `coding-standards`, `seo`, `tdd-workflow` and
  `.claude/rules/typescript.md`; versions in the plugin's `STACK.md`.
- Commands (from `web/`): `pnpm typecheck`, `pnpm lint`, `pnpm test`, `pnpm knip`, `pnpm build` (calls the Go
  API: start it first), `pnpm e2e`. Before saying done: `pnpm check`.
- `src/lib/api/schema.ts` is generated: `pnpm generate:api` after `api/openapi.yaml` changes; never edit it.
- A lint error from `boundaries/dependencies` means the code is in the wrong place: move it, don't widen the config.
