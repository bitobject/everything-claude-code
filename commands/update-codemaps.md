---
description: Regenerate docs/codemaps (backend modules and ports, web routes and rendering modes, data ownership) from the current code.
---

Use the **doc-updater** agent to refresh `docs/codemaps/backend.md`, `docs/codemaps/web.md` and
`docs/codemaps/data.md` from the code (packages and imports, route files, migrations). Keep each
under 150 lines, stamp them with the date and `git rev-parse --short HEAD`, and list any
discrepancy between code and approved docs instead of resolving it.
