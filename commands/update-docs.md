---
description: Sync README, CLAUDE.md and docs with the code — commands from Makefile/package.json, configuration from .env.example, API reference from api/openapi.yaml.
---

Use the **doc-updater** agent. Derive every command, env var and path from the source of truth
and check that each exists. Show the diff of each doc and the discrepancies you found.
