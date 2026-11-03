# REST, Bruno, PWA и документация Deploy/Revision/Discard
Status: ready-for-agent
Blocked by: 35_4

Часть 5 задачи 35. Всё, что делает решение видимым снаружи: маршруты, клиент, PWA, коллекции Bruno и документы решений.

Источник истины — `todo/backlog/35_projectfile_deploy_revision.md` (гриллинг 2026-11-03, Q1, Q3, Q11, Q12, Q13): это **спецификация**, а не отдельная работа.

## Что сделать

### REST (`contract/api/v0` — MINOR, `make schemas`)

- `GET /api/v0/projects/{id}/revision` (Runtime-нода) → `{number, deployed_at, content}`; 404, если Deploy не было.
- `GET /api/v0/projects/{id}/tag-states` (Runtime-нода) → снимок `id → {value, quality, seq}` для первого рендера Operation; дальше SSE `tag_updated`.
- `GET /api/v0/projects/{id}/deploy/preview` (Engineering) → `{current_revision (может отсутствовать), unchanged, dropped_widgets}`.
- `POST /api/v0/projects/{id}/deploy` → 201 `{number, deployed_at}`; 409 `nothing_to_deploy`; 500 при сбое копии у Engineering с честным текстом.
- `POST /api/v0/projects/{id}/discard` → 202 `{discarding: true}`; 409 без Revision.
- Событие `revision_created` в списке событий API; ошибки — RFC 7807; `routes()` и `requestBodies` (в тесте) пополнить, каждый маршрут покрыт Bruno-запросом.

### apiclient

- `ProjectClient`: `Deploy`, `DeployPreview`, `Discard`, `Revision`, `TagStates`; контрактные типы как есть.

### PWA

- **Project-режим:** рядом с переключателем — бейдж «Revision N · deployed at T» (или «не развёрнуто»), индикатор «есть неразвёрнутые изменения», кнопка Deploy (диалог: preview → подтверждение → ошибка сервера в toast) и кнопка Discard (подтверждение, отказ без Revision). Событие `revision_created` обновляет бейдж и индикатор.
- **Operation-режим:** читает только `GET …/revision` и `GET …/tag-states`; пустое состояние «проект не развёрнут»; перезагрузка по `revision_created` и на resync; content-события Draft и `library_*` больше не вызывают перезагрузку; WidgetType'ы — из вшитых в Revision копий (маппинг `projectv0` → структуры `livescene`).
- Тесты `ui/uitest`: Deploy-диалог показывает preview, отказ «нечего деплоить» виден, Operation на неразвёрнутом проекте показывает подсказку, а на развёрнутом — сцены Revision и не реагирует на правку Draft.

### Bruno

- Коллекция сервера: раздел `deploy` (preview → deploy → revision → tag-states → повторный deploy 409) **до** первого `PATCH …/value`; порядок `seq` сохраняется.
- Коллекция симулятора: деплой проекта перед запуском драйвера.
- Проверить статическую проверку (`TestBrunoCollection`): каждый новый маршрут покрыт, тела разбираются строго.

### Документация

- Новый **ADR 0013** «Deploy делает неизменяемую Revision; Draft сбрасывается Discard»: jsonb — истина, копии производные, номер выдаёт Runtime, сравнение «нечего деплоить» — с Revision, Discard как policy, состояния Tag живут по Revision.
- Поправки: ADR 0011 (мост убран, копии заполняются из Revision), ADR 0007 (пины в ProjectFile как провенанс), ADR 0008 если нужно (Discard — policy).
- `AGENTS.md`: Runtime-нода принимает Revision и отдаёт конфиг из неё; Operation читает Revision; мост удалён; ручной прогон симулятора требует Deploy.
- `GLOSSARY.md` — сверить формулировки Deploy/Revision/Discard/ProjectFile с реализацией; `CHANGELOG.md`.

## Проверка

- `make build`, `make test`, `make bruno_test`.
- Руками в браузере: Deploy → Operation показывает Revision; правка Scene в Draft не видна в Operation; повторный Deploy → 409; Discard возвращает Draft; консоль без ошибок.
- `make schemas` без диффа после коммита.

## Вне объёма

- Список ревизий и Rollback — задача 36; DraftChanges — 38; runtime-сущности и непустой `dropped_widgets` — 48; EditLock — 37; сетевой `RuntimeNode` — 52.
