---
description: Find and safely remove dead code, unused dependencies and duplication in backend/ and web/, in verified batches.
argument-hint: "[backend | web | path]"
---

Use the **refactor-cleaner** agent on: $ARGUMENTS (if empty: backend/ and web/).

Show the candidate list (safe / check / keep) before deleting anything and wait for the user's
go-ahead. Then remove in batches, running build and tests after each batch.
