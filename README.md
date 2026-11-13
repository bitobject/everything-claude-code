# ecc: everything-claude-code для Go + Next.js

Адаптация форка [WorldFlowAI/everything-claude-code](https://github.com/WorldFlowAI/everything-claude-code)
(оригинал [affaan-m/everything-claude-code](https://github.com/affaan-m/everything-claude-code), MIT)
под стек:

- **бэкенд на Go 1.27**: строгая Clean Architecture, модули со слоями внутри
  (`entity → usecase → repository/delivery/gateway`), PostgreSQL через pgx + sqlc + goose,
  фоновые задачи на River;
- **API contract-first**: `api/openapi.yaml` (OpenAPI 3.1), Go-сервер (oapi-codegen, strict + `net/http`)
  и TS-клиент (openapi-typescript + openapi-fetch) генерируются из контракта;
- **фронт на Next.js 16 App Router**, рассчитанный на SEO (Яндекс + Google).

Все решения и версии (проверены 2026-11-12) собраны в **[STACK.md](STACK.md)**. Если файл
плагина расходится с ним, прав STACK.md.

## Что внутри

| Тип | Состав |
|---|---|
| Агенты | `architect`, `planner`, `code-reviewer` (маршрутизатор), `go-reviewer`, `database-reviewer`, `typescript-reviewer`, `security-reviewer`, `seo-specialist`, `tdd-guide`, `go-build-resolver`, `build-error-resolver` (TS/Next), `e2e-runner`, `refactor-cleaner`, `doc-updater` |
| Скиллы | `go-clean-architecture` (+ проверенный эталонный модуль в `reference/`), `golang-patterns`, `golang-testing`, `postgres-patterns`, `openapi-contract-first`, `nextjs-patterns`, `seo`, `coding-standards` (TS), `security-review`, `tdd-workflow`, `verification-loop`, `eval-harness`, `strategic-compact`, `setup-project` |
| Команды | `/ecc:plan`, `/ecc:tdd`, `/ecc:verify`, `/ecc:code-review`, `/ecc:go-review`, `/ecc:go-test`, `/ecc:go-build`, `/ecc:build-fix`, `/ecc:new-module`, `/ecc:e2e`, `/ecc:test-coverage`, `/ecc:refactor-clean`, `/ecc:update-docs`, `/ecc:update-codemaps`, `/ecc:learn`, `/ecc:checkpoint`, `/ecc:orchestrate`, `/ecc:eval` |
| Правила | `rules/workflow.md`, `go.md`, `sql.md`, `openapi.md`, `typescript.md`: копируются в `.claude/rules/` через `/ecc:setup-project`, потому что плагины не могут поставлять правила сами |
| Хуки | в проектах с маркером `.claude/ecc.json`: строка с путями к STACK.md, эталону и скиллам на старте сессии, после `/compact` и при запуске каждого субагента; запрет правок сгенерированного кода; после правки `.go`: goimports/gofmt, `go vet` пакета, поиск отладочного вывода; после правки TS: prettier, tsc и поиск `console.log` (только локальные инструменты, без npx); перед завершением ответа: проверка отладочного вывода в добавленных строках |

## Эталон

`skills/go-clean-architecture/reference/` — компилируемый срез архитектуры. В нём:
- контракт с правилами линта;
- полный модуль `leads` со всеми слоями и публичным контрактом;
- тонкий модуль `notifications`, который реагирует на событие `leads`;
- миграции под двумя ролями, sqlc;
- два composition root (`cmd/api`, `cmd/worker`);
- конфиги линтеров и Makefile.

13.11.2026 на PostgreSQL 18 прогнан `make ci`. В него входят:
- golangci-lint (0 замечаний), go-arch-lint, vacuum, squawk;
- `go test -race -shuffle=on` со всеми тестами на базе;
- govulncheck, `go fix -diff`;
- контроль ломающих изменений контракта.

Нарочно внесённые нарушения ловятся. Подробности — в README эталона. Скиллы ссылаются на эталон, а не
копируют код.

## Как знания доходят до агентов

Проверено на Claude Code 2.1.323 (13.11.2026). План и результаты проверок:
[`docs/plans/subagent-knowledge-delivery.md`](docs/plans/subagent-knowledge-delivery.md).

| Канал | Кто получает | Что |
|---|---|---|
| CLAUDE.md и `.claude/rules` проекта, ваши `~/.claude/rules` | основная сессия и субагенты, кроме встроенных Explore и Plan; правила по путям — после чтения подходящего файла | правила проекта |
| поле `skills:` агента | этот агент, полным текстом при старте | 1–2 главных скилла роли |
| список скиллов и инструмент Skill | основная сессия и все агенты плагина: они наследуют инструменты (у них нет `tools:`) | остальные скиллы, по необходимости |
| хук `context-hint.sh` (SessionStart, SubagentStart) | основная сессия (и после `/compact`) и все субагенты, включая Explore и Plan; только в репозиториях с `.claude/ecc.json` в корне, в том числе при запуске из подпапки | пути к STACK.md и эталону, указание загружать скиллы `ecc:*` через Skill |
| `${CLAUDE_PLUGIN_ROOT}` в тексте агентов, `SKILL.md` и команд; `${CLAUDE_SKILL_DIR}` только в `SKILL.md` | Claude Code подставляет абсолютные пути | где лежат STACK.md и эталон |

Почему так:
- Один доступ к скиллам не помогает. В проверке ревьюер, в тексте которого скиллы не названы, не
  вызвал ни одного (0 из 3 прогонов). Когда скиллы названы в тексте агента или их подсказывает хук,
  он вызывал нужные (3 из 3). Поэтому каждый агент называет свои скиллы как `ecc:<имя>`.
- Explore и Plan не получают CLAUDE.md и правил, поэтому строка хука — единственное, что они узнают
  о плагине.
- Ограничения ролей задаются через `disallowedTools`: ревьюерам запрещены Write, Edit, NotebookEdit и
  Agent, остальным агентам — Agent. Указывать `Skill` в `tools:` — устаревший способ; скиллы
  предзагружаются полем `skills:`.
- «Только чтение» у ревьюеров — указание в их тексте, а не запрет: Bash и MCP-инструменты сессии
  у них есть. Их ограничивают только права основной сессии, потому что агенты плагина игнорируют
  `permissionMode`.
- Во вспомогательных файлах скиллов (их читают через Read) переменные не подставляются, поэтому
  `${CLAUDE_…}` используется только в `SKILL.md`, командах и агентах.

## Установка

```bash
claude plugin marketplace add /root/lead_new/.localenv/plugins/everything-claude-code
claude plugin install ecc@ecc-local
```

Затем в проекте: `/ecc:setup-project`. Кроме прочего, он создаёт маркер `.claude/ecc.json`, без
которого хук `context-hint.sh` молчит. В проекте, настроенном раньше, запустите его ещё раз или
создайте файл вручную: `{"plugin": "ecc", "version": "<версия из .claude-plugin/plugin.json>"}`.

Хуки можно частично отключить переменными окружения `ECC_GO_VET=0` и `ECC_TSC=0`, например на время
большого рефакторинга.

## Разработка плагина

- Тесты хуков: `bash tests/hooks.test.sh` и `bash tests/context-hint.test.sh`.
- Агенты, скиллы, команды (frontmatter, ссылки на STACK.md и скиллы): `bash tests/agents.test.sh`.
- Живой тест доставки знаний: `bash tests/context-delivery.sh`. Он делает настоящие вызовы модели
  (haiku, около $0,02 за прогон; лимит `BUDGET`, по умолчанию $1, действует на каждый из трёх
  вызовов `claude -p`). Запускайте его после обновления
  Claude Code и после изменений агентов, скиллов или хуков.
- Проверка манифестов и компонентов: `claude plugin validate .`.
- После изменений: `claude plugin marketplace update ecc-local && claude plugin update ecc@ecc-local`.

## Что убрано из форка и почему

- JS-хуки на Node: в них был устаревший синтаксис matcher (`tool == "Bash" && …`), который текущий
  Claude Code не понимает. Среди них был блок dev-сервера вне tmux и запрет создавать любые `.md`
  (он ломал `docs/`).
- Самописная «память» сессий (`~/.claude/sessions`) и continuous-learning: у Claude Code есть встроенная
  память, а для уроков есть `/ecc:learn`.
- `backend-patterns` (Node/Express), `clickhouse-io`, `project-guidelines-example`, `setup-pm`,
  `mcp-configs`, `WORLDFLOWAI.md` (Rust/Python-проекты WorldFlowAI).
