# growctl: манифесты Device и Environment
Status: needs-triage
Priority: 3

Слой C. Термины — `GLOSSARY.md` (Device, Environment, Environment Device).

Выделено из задачи 34 (части A) 2026-11-01: REST для устройств и окружений готов (часть 4), а манифестов growctl нет. **Спецификация не готова** — проектировать вместе с инженером.

## Что нужно

- **Манифест Device**: имя (уникально в проекте), комментарий, протокол, конфигурация; привязка Tag'ов к Device (у Tag'а — поставщик).
- **Манифест Environment**: имя, комментарий, и для каждой привязанной Device — параметры связи.
- Токены **не** в манифесте: секрет, он выдаётся через API и в файле не хранится.

## Что это значит по контракту

- Новый `kind` в `contract/manifest/v0` (сейчас SchemaVersion 0.5; kinds — Tag, Project, Library, WidgetType) — например `Device` и `Environment`.
- Своя «Since»-версия для `requireAPIVersion` (как `projectsSince`, `librariesSince` и `libraryImportsSince` в `internal/growctl/manifest.go`): старые манифесты без новых kind'ов остаются рабочими.
- Bump `SchemaVersion()` контракта манифестов + `make schemas` (правило 4 AGENTS.md).
- `internal/growctl/manifest.go` — разбор и валидация новых kind'ов; `plan.go` — планирование операций; `client.go` — вызовы новых маршрутов REST.
- Привязка Tag→Device ждёт REST-поля поставщика (задача 68); без неё манифест Device сможет задать только само устройство и его конфигурацию.
- Тесты: `internal/growctl/manifest_test.go`, `growctl_e2e_test.go` (против настоящего API), статическая сверка Bruno.

## Связанное

- Задача 34, часть 4 — REST, к которому обращается growctl.
- Задача 68 — REST-поле поставщика Tag'а; от неё зависит пункт «привязка Tag'ов к Device».
- Задача 69 — типизированный конфиг `simulation`: манифест должен валидировать конфигурацию так же, как домен.
- Задача 14 (`todo/done/14_growctl.md`) — исходная постановка growctl.
- `contract/manifest/v0/manifest.go` — контракт манифестов; `docs/adr/0005-contract-schema-versioning.md` — правило версионирования.

## Comments

- 2026-11-01 (grilling задачи 34): выделено из задачи 34 вместе с UI; спецификация — за инженером.
- 2026-11-07: после 72_8 growctl работает без Check in/out; apply правит Draft, публикация — save → release → deploy. Выполнять после 72_8, чтобы не переписывать команды дважды.
- 2026-11-15: сверено с кодом. Версия манифестов уже 0.5 (`libraryImports`), Release больше нет: путь — `apply` → `save`, `deploy --commit N`, а импорт Library — номер сохранённого состояния в `spec.libraryImports`. Зависимости 34_4 и 72_8 закрыты — `Blocked by` убран.
