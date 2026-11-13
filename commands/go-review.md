---
description: Review Go changes in backend/ only — dependency rule, idiomatic Go 1.27, errors, concurrency, tests — with go vet, golangci-lint and go-arch-lint output.
argument-hint: "[git range | package path]"
---

Use the **go-reviewer** agent on: $ARGUMENTS (if empty: the uncommitted Go changes in backend/).

If migrations, `queries/*.sql` or repository adapters changed, also run **database-reviewer**
on them. Report findings by severity with file:line and a concrete fix; don't edit files.
