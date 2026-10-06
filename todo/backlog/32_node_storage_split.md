# Разделение узлов и хранилищ, Tag definition/state, Origin
Status: ready-for-human
Blocked by: 31

Слой A. Архитектура — ADR 0002; термины — `CONTEXT.md` (Engineering node, Runtime node, Origin).

- Две схемы Postgres: `engineering.*` (Draft: Scenes, Widgets, определения Tag, Libraries) и `runtime.*` (текущие значения/Quality тегов, runtime-сущности; дальше — Revisions, Journal, Checkpoints, Device Tokens). Отдельные цепочки goose-миграций, отдельные строки подключения (в dev — одна БД). `project_id` (задача 31) — в обеих схемах.
- Никаких запросов из одной схемы в другую. Engineering-код обращается к Runtime только через интерфейс (сейчас — in-process реализация, позже — HTTP API, задача 52).
- Tag: определение (имя, TagType) — в engineering; значение и Quality — в runtime. `PATCH /tags/{id}/value` работает с runtime.
- Поле Origin (`Project`/`Runtime`) у Tag и Widget; enum без Unknown (ADR 0001).
- Имя Tag уникально среди всех Tag проекта независимо от Origin.
- Миграция существующих данных dev-БД и seed; тесты репозиториев (testcontainers) на обе схемы; Bruno; `CHANGELOG.md`.

## Comments

- 2026-10-04: бывший тикет 01 задачи 36 (`36_project_undo_redo_history`, расформирована). Решения, на которых стоит: Q2 (runtime-сущности Tag/Widget создают Device/интеграции и Operator; у WidgetType нет Origin), Q9 (в Draft/Revision — только определение Tag, значение и Quality — runtime), Q20 (существующие таблицы становятся Draft), Q26–Q28 (один бинарник, роли Engineering/Runtime node, две схемы БД, связь только через API Runtime node).
- 2026-10-06 (DDD-ревью задачи 20): разделение Tag — не только хранилище, но и модель. Определение Tag и его значение с Quality — две границы согласованности: определение — агрегат Draft с ожидаемой Version и edit conflict, значение — состояние Runtime с единственным писателем (Device или людьми), без ожидаемой Version. `devicelink` перестаёт отслеживать Version и повторять запись после 409 (ADR 0004, контракт API). Запись значения не порождает строку outbox на каждое изменение; запись в историю — Journal (задача 42). До этой задачи сверку Version при записи значения делает сервис (задача 20 её в агрегат не переносит).
- 2026-10-07 (code review задачи 20, замечание 8): `Tag` пока изменяемый (в отличие от `Scene` и `WidgetType`): `SetValue` меняет сам тег и дописывает `tag_updated` с версией «текущая + 1». Два `SetValue` до `Save` дадут в outbox два `tag_updated` с одной Version и разными значениями; подписчик, отбрасывающий повторы по Version (симулятор), оставит первое, а в БД — последнее. Сейчас не проявляется: `SetTagValueByID` вызывает `SetValue` один раз. При разделении Tag: запись значения не должна давать нескольких событий с одной Version (или не давать строк outbox вовсе, как решено выше); определение Tag — неизменяемый агрегат, как Scene и WidgetType. В приёмку — тест: несколько записей значения подряд не дают двух событий, претендующих на одну Version.
