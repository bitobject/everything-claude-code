# Plan: знания доходят до субагентов, агенты в текущем формате

Status: approved 2026-11-13
Request: «Доставить знания субагентам… сделать, чтобы агенты были написаны правильно, а не устаревшим
методом» (13.11.2026).
Touches: `agents/*.md` (14), `hooks/hooks.json`, `hooks/scripts/context-hint.sh` (новый),
`skills/*/SKILL.md` (ссылки на `STACK.md` и `reference/`), `commands/plan.md`,
`skills/setup-project/SKILL.md`, `tests/`, `README.md` · ADRs: none

## Goal
В проекте, настроенном через `/ecc:setup-project`, любой субагент (14 агентов плагина и встроенные
Explore, Plan, general-purpose) с первого хода знает, где лежат `STACK.md`, эталон и скиллы, и сам
загружает нужные ему скиллы. Основная сессия получает ту же строку и сохраняет её после `/compact`.
Агенты описаны так, как требует текущая документация: инструменты наследуются, ограничения роли
задаются через `disallowedTools`, скиллы предзагружаются полем `skills:`, а не через `Skill` в `tools:`.

## Что проверено перед планом (Claude Code 2.1.323, 13.11.2026)
Проверял на тестовых плагинах в scratchpad через `claude -p`. Доказательства взяты из журналов хуков
и записей диалогов субагентов, а не только из ответов моделей.

| Факт | Как проверено |
|---|---|
| Субагенты плагина получают CLAUDE.md проекта, `.claude/rules`, правила пользователя, а после Read — правила по путям | метки в контексте трёх вариантов агента |
| `${CLAUDE_PLUGIN_ROOT}` в тексте агента заменяется абсолютным путём | там же |
| Агент со списком `tools:` не получает ни Skill, ни списка скиллов, ни MCP. Без `tools:` получает всё; `disallowedTools` убирает перечисленное | там же |
| `skills:` вставляет полный текст скилла. Работают короткое имя (скилл своего плагина), `probe2:probe-skill` и `ecc:golang-patterns` (скилл другого плагина) | метки и уникальная строка скилла в записи диалога |
| Текст SessionStart-хука до субагентов не доходит | метки |
| SubagentStart срабатывает для Explore, Plan, general-purpose, claude и агентов плагинов, и его текст до них доходит. На входе есть `agent_type` и `cwd` | журнал хука и записи диалогов |
| Explore и Plan не получают ни CLAUDE.md, ни правил пользователя, так что хук для них единственный канал | в их записях нет метки CLAUDE.md |
| SessionStart срабатывает с `source=compact`, и его текст оказывается в контексте после сводки | журнал хука и ответ модели после `/compact` |
| Ревьюер с доступом к скиллам: если скиллы нигде не упомянуты, Skill не вызван ни в одном из 3 прогонов. Если скиллы названы в тексте агента — вызваны в 3 из 3. С общей подсказкой из хука — тоже 3 из 3, причём go-clean-architecture и golang-patterns он выбрал сам | вызовы Skill в записях диалогов; opus, одна задача, по 3 прогона |
| Даже без скиллов ревьюер знал часть правил (apperr, Clock, FOR NO KEY UPDATE): их принесли правила по путям из `~/.claude/rules/go.md`. Замечания, знание для которых есть только в скиллах (Restore, теги в entity), сделали только варианты, вызвавшие скиллы | текст отчётов |

Документация (code.claude.com/docs/en/sub-agents, …/plugins/components):
- «To preload Skills into context, use the `skills` field rather than listing `Skill` here». В схеме
  CLI 2.1.323 указание `Skill` в `tools` помечено как deprecated.
- Агенты плагина игнорируют поля `permissionMode`, `hooks`, `mcpServers` и `initialPrompt`.
- `skills:`: «The full content of each listed skill is injected… up to the first 32 distinct names».
  Скилл с `disable-model-invocation: true` предзагрузить нельзя.
- `commands/`: «the older format, and skills supersede them for new work».

## Acceptance criteria
- AC1. В репозитории с `.claude/ecc.json` в корне, в том числе когда Claude Code запущен из его
  подпапки, хук на SubagentStart с любым `agent_type`, включая `Explore`,
  печатает JSON `hookSpecificOutput` с `hookEventName: SubagentStart` и `additionalContext`. В тексте
  есть абсолютные пути `<plugin root>/STACK.md` и `<plugin root>/skills/go-clean-architecture/reference/`
  и указание загружать скиллы `ecc:*` через Skill. Test: `tests/context-hint.test.sh`.
- AC2. То же для SessionStart при каждом значении `source`: startup, resume, clear, compact, fork.
  Test: `tests/context-hint.test.sh`.
- AC3. Хук ничего не печатает и выходит с кодом 0, если маркера нет, если маркер лежит только выше
  корня репозитория и если на вход пришёл битый JSON. Test: `tests/context-hint.test.sh`.
- AC4. Текст хука короче 1 000 символов. Test: `tests/context-hint.test.sh`.
- AC5. Ни у одного агента нет `tools:`. `disallowedTools` совпадает с таблицей ролей, и у каждого
  агента в нём есть `Agent`. Test: `tests/agents.test.sh`.
- AC6. `skills:` совпадает с таблицей ролей. Каждый скилл из списка есть в `skills/`, у него нет
  `disable-model-invocation: true`, и в списке не больше 32 имён. Test: `tests/agents.test.sh`.
- AC7. В агентах, в `SKILL.md` и в командах нет упоминаний `STACK.md` без префикса
  `${CLAUDE_PLUGIN_ROOT}/`, а каждый упомянутый скилл `ecc:<имя>` существует. Вспомогательные файлы
  скиллов, `rules/` и `examples/` (последние копируют в проекты) не используют переменные `${CLAUDE_*}`:
  в них ничего не подставляется, поэтому там остаётся просто `STACK.md`, а путь к нему сообщает хук.
  Test: `tests/agents.test.sh`.
- AC8. `/ecc:setup-project` при любой цели (`project` и `user`) создаёт в репозитории
  `.claude/ecc.json` и упоминает его в отчёте. Test: `tests/agents.test.sh` проверяет шаг 7 и отчёт
  шага 8; ручной запуск описан в результатах шага 1.
- AC9. Живой тест `tests/context-delivery.sh` на текущем Claude Code, в одноразовом проекте с
  маркером, проверяет три вещи:
  - (a) в записи диалога `ecc:go-reviewer` есть абсолютный путь к `STACK.md` (и в тексте агента, и в
    тексте предзагруженного `go-clean-architecture`) и строка хука;
  - (b) у Explore есть строка хука;
  - (c) в основной сессии строка хука есть после `/compact`.

  Тест печатает PASS или FAIL и стоимость прогона. Test: ручной запуск `bash tests/context-delivery.sh`
  с лимитом `--max-budget-usd 1`.
- AC10. `claude plugin validate` проходит. `tests/hooks.test.sh` (22 старых проверки) и
  `tests/context-hint.test.sh` зелёные.

## Таблица ролей

| Агент | Роль | `disallowedTools` | `skills:` (предзагрузка) |
|---|---|---|---|
| code-reviewer | ревью | Write, Edit, NotebookEdit, Agent | openapi-contract-first (A1) |
| go-reviewer | ревью | Write, Edit, NotebookEdit, Agent | go-clean-architecture, golang-patterns |
| database-reviewer | ревью | Write, Edit, NotebookEdit, Agent | postgres-patterns |
| typescript-reviewer | ревью | Write, Edit, NotebookEdit, Agent | nextjs-patterns |
| security-reviewer | ревью | Write, Edit, NotebookEdit, Agent | security-review |
| seo-specialist | аудит, код не правит | Write, Edit, NotebookEdit, Agent | seo |
| architect | пишет ADR и docs | Agent | go-clean-architecture |
| planner | пишет план | Agent | go-clean-architecture |
| tdd-guide | реализация | Agent | tdd-workflow, go-clean-architecture |
| go-build-resolver, build-error-resolver, e2e-runner, doc-updater, refactor-cleaner | правки | Agent | нет: список скиллов и хук |

Остальные скиллы агент видит в общем списке. В тексте каждого агента они названы `ecc:<имя>` с
указанием загрузить их через Skill, если задача их касается.

## Scope
- In:
  - frontmatter и ссылки во всех 14 агентах;
  - хук на SessionStart и SubagentStart;
  - маркер в `setup-project`;
  - пути к `STACK.md` и `reference/` в скиллах и `commands/plan.md`;
  - статические тесты и живой тест;
  - раздел в README.
- Out:
  - модели агентов (у всех `model: opus`): это пункт G2, он отложен;
  - перенос 18 команд из `commands/` в `skills/`: документация называет `commands/` старым
    форматом, но это отдельный план;
  - разделение `nextjs-patterns` (799 строк, около 42 тыс. символов): отдельная задача;
  - кейсы `claude plugin eval`: отдельно, после этого плана;
  - вставка CLAUDE.md в Explore и Plan: Claude Code намеренно им его не даёт, поэтому хук передаёт им
    только указатель.

## Open questions
Нет: три решения приняты 13.11.2026.

## Decisions
- D1. Инструменты наследуются (`tools:` удалён), ограничения роли задаются через `disallowedTools`. —
  пользователь, 13.11.2026.
- D2. Текст хука подключается только в проектах с маркером `.claude/ecc.json`, который создаёт
  `setup-project`. — пользователь, 13.11.2026.
- D3. Субагентам плагина запрещён `Agent`. Поведение от этого не меняется: сейчас ни у одного агента
  нет `Agent` в `tools:`. — пользователь, 13.11.2026.
- D4. В `skills:` скиллы указываются короткими именами (`go-clean-architecture`), как в документации.
  В тексте агентов они названы `ecc:<имя>`, как в общем списке скиллов. Причина в проверке: когда
  скиллы не упомянуты, их не вызывали (0 из 3), когда упомянуты — вызывали (3 из 3). — Claude, по
  результатам проверки.
- D5. Один скрипт обслуживает оба события, без matcher. Тогда он срабатывает и после `/compact`, и для
  встроенных агентов (проверено). — Claude, по результатам проверки.
- D6 (шаг 1). Тесты хука вынесены в отдельный `tests/context-hint.test.sh`, а не в
  `tests/hooks.test.sh`. Причина: `protect-generated.sh` запрещает правку `tests/hooks.test.sh` и
  считает его сгенерированным, потому что в первых 30 строках есть фразы-образцы «auto-generated by»
  и «Do not make direct changes» внутри `printf`. Это ложное срабатывание: хук ищет маркеры в любой
  позиции строки, а не только в комментарии-заголовке. Обходить хук не стали. Исправление вынесено
  отдельным предложением. — Claude, 13.11.2026.
- D7 (ревью). Хук находит маркер в корне git-репозитория и тогда, когда Claude Code запущен из
  подпапки. Проверка показала, что `CLAUDE_PROJECT_DIR` — это папка запуска: после `cd web && claude`
  строки не было. Допущение A3 изменено. — пользователь, 13.11.2026.
- D8 (ревью). MCP-инструменты у ревьюеров пока оставлены, риск R6 принят. — пользователь, 13.11.2026.

## Assumptions
- A1. code-reviewer предзагружает `openapi-contract-first`, потому что контракт он проверяет сам.
- A2. planner и architect по наследованию получат Bash, которого у них сейчас нет. Это нужно для
  правила «проверяй текущие версии» (`go list -m -versions`, `npm view`). Права основной сессии
  действуют и для них.
- A3. Маркер `.claude/ecc.json` содержит `{"plugin": "ecc", "version": "<версия из plugin.json>"}` и
  коммитится вместе с проектом. Хук ищет его в корне git-репозитория. Корень он определяет от
  `CLAUDE_PROJECT_DIR` (туда Claude Code ставит папку запуска, это может быть и `web/`), а без неё —
  от `cwd`; вне git маркер ищется в самой этой папке. Выше корня репозитория маркер не ищется.
  Изменено после ревью (D7).
- A4. Текст хука на английском, как все тексты плагина, около 450 символов:
  > ecc harness: stack defaults and pinned versions are in `<root>/STACK.md`; an accepted ADR in
  > docs/adr/ wins over it. Compiling reference app: `<root>/skills/go-clean-architecture/reference/`.
  > This project's conventions live in the ecc:* skills: before you start, load the ones that match
  > your task with the Skill tool, unless they are already in your context.
- A5. Чтобы не платить за opus, живой тест задаёт `CLAUDE_CODE_SUBAGENT_MODEL=haiku` и
  `CLAUDE_CODE_SUBAGENT_MODEL_FORCE=1`. В CLI 2.1.323 FORCE — это флаг: с ним модель из frontmatter
  агента не учитывается, а берётся из `CLAUDE_CODE_SUBAGENT_MODEL`. Проверено: субагенты работали на
  claude-haiku-5-5.

## Steps
- [x] 1. hooks: создать `hooks/scripts/context-hint.sh` на основе `lib.sh`; в `hooks/hooks.json`
  добавить SessionStart и SubagentStart с timeout 10; в `skills/setup-project/SKILL.md` добавить
  создание маркера. Test: `bash tests/context-hint.test.sh` (D6). Covers: AC1–AC4, AC8.
  Ручной запуск `/ecc:setup-project project` (haiku, `-p`, одноразовый репозиторий): модель дошла до
  шага 7 и определила версию `2.0.0-go-nextjs`, но Claude Code в неинтерактивном режиме запретил
  запись в `.claude/`, и маркера, и правил. В обычной сессии такую запись подтверждает пользователь.
  Цель `user` вручную не запускалась: она пишет правила в настоящий каталог настроек.
- [x] 2. frontmatter агентов: привести 14 файлов к таблице ролей; создать `tests/agents.test.sh`.
  Test: `bash tests/agents.test.sh`, `claude plugin validate .`. Covers: AC5, AC6, AC10.
- [x] 3. текст агентов: заменить ссылки на `${CLAUDE_PLUGIN_ROOT}/STACK.md` и
  `${CLAUDE_PLUGIN_ROOT}/skills/go-clean-architecture/reference/`; назвать скиллы `ecc:<имя>` с
  указанием загружать их через Skill. Test: проверки AC7 для `agents/` в `tests/agents.test.sh`.
  Covers: AC7.
- [x] 4. скиллы и команды: те же пути в 9 скиллах, которые упоминают `STACK.md`, и в
  `commands/plan.md`; на собственные файлы скилла ссылаться через `${CLAUDE_SKILL_DIR}`. Test:
  проверки AC7 для `skills/` и `commands/`. Covers: AC7.
- [x] 5. живой тест и документация: создать `tests/context-delivery.sh`, добавить в `README.md`
  раздел «Как знания доходят до агентов». Test: `bash tests/context-delivery.sh` → PASS.
  Covers: AC9.
  Результат 13.11.2026: 9 из 9, $0,016, субагенты на claude-haiku-5-5. Первый запрос go-reviewer
  занял 30 295 токенов. Прогон без маркера: падают ровно 3 проверки хука, остальные 6 проходят.

## Risks
- R1. Цена. Каждый субагент теперь получает схемы инструментов и список скиллов. По моей оценке по
  `/context` это около 25 тыс. токенов, бо́льшая часть которых кешируется. В проверке один прогон
  ревьюера на opus стоил около $0,40. Что делаем: замеряем в шаге 5 и предзагружаем только
  1–2 скилла. Замер (шаг 5): первый запрос go-reviewer — 30 295 токенов, включая два
  предзагруженных скилла (около 37 тыс. символов).
- R2. Каждый запуск typescript-reviewer будет загружать `nextjs-patterns` (около 10 тыс. токенов). Что
  делаем: разделение скилла вынесено в отдельную задачу.
- R3. Возврат текста SessionStart-хука после `/compact` в документации не описан и проверен только на
  2.1.323. Что делаем: запускаем живой тест после каждого обновления Claude Code.
- R4. SubagentStart срабатывает и тогда, когда работу субагента продолжают через SendMessage, поэтому
  строка повторится. Это около 450 символов и ничему не мешает.
- R5. У ревьюеров остаётся Bash, так что через shell они по-прежнему могут менять файлы, как и сейчас.
  Действуют права основной сессии: агенты плагина игнорируют `permissionMode`.
- R6. Субагентам станут доступны MCP-инструменты окружения, например запись в claude.ai Docs. Права
  основной сессии на них тоже действуют. Если нужно, ревьюерам можно запретить `mcp__claude_ai_*`;
  в план это не включено, потому что имена серверов у каждого пользователя свои.
- R7. В уже настроенных проектах маркера нет, поэтому строки там не будет, пока не запустить
  `/ecc:setup-project` ещё раз или не создать файл вручную.
- R8. Живой тест тратит деньги: прогон с haiku стоит около $0,016. Лимит `BUDGET` (по умолчанию $1)
  действует на каждый из трёх вызовов `claude -p`, так что в худшем случае тест обойдётся в $3.
