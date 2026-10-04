# Разделение узлов и хранилищ, Tag definition/state, Origin
Status: ready-for-human
Blocked by: 25

Слой A. Архитектура — ADR 0002; термины — `CONTEXT.md` (Engineering node, Runtime node, Origin).

- Две схемы Postgres: `engineering.*` (Draft: Scenes, Widgets, определения Tag, Libraries) и `runtime.*` (текущие значения/Quality тегов, runtime-сущности; дальше — Revisions, Journal, Checkpoints, Device Tokens). Отдельные цепочки goose-миграций, отдельные строки подключения (в dev — одна БД). `project_id` (задача 25) — в обеих схемах.
- Никаких запросов из одной схемы в другую. Engineering-код обращается к Runtime только через интерфейс (сейчас — in-process реализация, позже — HTTP API, задача 46).
- Tag: определение (имя, TagType) — в engineering; значение и Quality — в runtime. `PATCH /tags/{id}/value` работает с runtime.
- Поле Origin (`Project`/`Runtime`) у Tag и Widget; enum без Unknown (ADR 0001).
- Имя Tag уникально среди всех Tag проекта независимо от Origin.
- Миграция существующих данных dev-БД и seed; тесты репозиториев (testcontainers) на обе схемы; Bruno; `CHANGELOG.md`.

## Comments

- 2026-10-04: бывший тикет 01 задачи 36 (`36_project_undo_redo_history`, расформирована). Решения, на которых стоит: Q2 (runtime-сущности Tag/Widget создают Device/интеграции и Operator; у WidgetType нет Origin), Q9 (в Draft/Revision — только определение Tag, значение и Quality — runtime), Q20 (существующие таблицы становятся Draft), Q26–Q28 (один бинарник, роли Engineering/Runtime node, две схемы БД, связь только через API Runtime node).
