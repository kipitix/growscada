# Разделение узлов и хранилищ, Tag definition/state, Origin

Type: task

- Две схемы Postgres: `engineering.*` (Draft: WidgetTypes, Scenes, Widgets, определения Tag) и `runtime.*` (текущие значения/Quality тегов, runtime-сущности; дальше — Revisions, Journal, Checkpoints). Отдельные цепочки goose-миграций, отдельные строки подключения (в dev — одна БД).
- Никаких запросов из одной схемы в другую. Engineering-код обращается к Runtime только через интерфейс (сейчас — in-process реализация, позже — HTTP API).
- Tag: определение (имя, TagType) — в engineering; значение и Quality — в runtime. `PATCH /tags/{id}/value` работает с runtime.
- Поле Origin (`Project`/`Runtime`) у Tag и Widget; enum без Unknown (ADR 0001).
- Имя Tag уникально среди всех Tag независимо от Origin.
- Миграция существующих данных dev-БД и seed; тесты репозиториев (testcontainers) на обе схемы; Bruno; CHANGELOG.md.
