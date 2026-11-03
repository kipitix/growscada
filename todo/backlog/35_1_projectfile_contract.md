# Формат ProjectFile: контракт `project/v0`
Status: ready-for-agent
Blocked by: 33

Часть 1 задачи 35. **Только формат:** типы `contract/project/v0`, `SchemaVersion`, сгенерированные схемы. Поведения здесь нет — ни миграций, ни Deploy.

Источник истины — `todo/backlog/35_projectfile_deploy_revision.md` (гриллинг 2026-11-03, Q2 и Q14): это **спецификация**, а не отдельная работа.

## Что сделать

### Типы `contract/project/v0`

- `Header` — `{schema_version, project_id}`. Имени проекта нет: Runtime и так знает его из `public.projects`, а переименование не должно требовать Deploy (Q14). Документ `ProjectFile` встраивает `Header`; `Revision` имеет ту же форму (ADR 0005), поэтому отдельного типа для неё не нужно.
- `ProjectFile` — полный снимок конфигурации (Q2), списки без порядка (порядок не значим, канонизирует сборщик в части 4):
  - `tags []TagDefinition` — `{id, name, type, supplier_device_id}`, `supplier_device_id` опускается, когда значение пишут люди;
  - `devices []Device` — `{id, name, comment, protocol, config}`; `config` — `json.RawMessage`, JSON под протокол;
  - `environments []Environment` — `{id, name, comment}`;
  - `environment_devices []EnvironmentDevice` — `{environment_id, device_id, params}` (пара без собственного ID);
  - `scenes []Scene` — `{id, name, width, height, background_html, widgets []Widget}`;
  - `widgets` — `{id, name, position{x,y,z}, size{width,height}, origin{x,y}, rotation{degrees}, type_id, labels, port_bindings[{port_name, tag_id}]}`;
  - `widget_types []WidgetType` — копия содержимого используемого WidgetType из закреплённого Release: `{id, library_id, name, html_template, script, script_language, default_width, default_height, input_ports[{name, description, type_hint}]}`;
  - `library_pins []LibraryPin` — `{library_id, release}`: провенанс, по которому Discard восстановит набор пинов.
- Не входят: состояния Tag (значение, Quality, `seq`), Device Tokens, версии агрегатов, трансформ-матрица Widget'а (производная), история и Journal.

### Версия и схемы

- `projectv0.SchemaVersion()` → **0.2** (MINOR: формат только расширяется, старый `Header` остаётся читаемым).
- `make schemas`: в `schemas/project/` появляется `0.2.json`, `0.1.json` замораживается в `frozen.sha256`.
- `schema_test.go` пакета — как у остальных контрактов: типы и закоммиченная схема совпадают.

## Проверка

- `make build`, `make test` проходят; `make schemas` не оставляет незакоммиченных изменений.
- `go vet ./contract/project/...`; `contract/project/v0` не импортирует ничего, кроме `contract` и `encoding/json`.

## Вне объёма

- `contract/api/v0` (эндпоинты и событие `revision_created`) — часть 5.
- Миграции и репозитории — часть 2; поведение Deploy — части 3–4.
