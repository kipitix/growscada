# UI: устройства, окружения и токены в режиме Project
Status: needs-triage
Priority: 3
Blocked by: 34_4, 72_6

Слой E. Термины — `GLOSSARY.md` (Device, Environment, Environment Device, Device Token).

Выделено из задачи 34 (части A) 2026-11-01: REST для устройств, окружений и токенов готов (часть 4), а интерфейса нет. **Спецификация не готова** — проектировать вместе с инженером.

## Что нужно в интерфейсе

- **Создание и правка Device**: имя (уникально в проекте), комментарий, протокол (`simulation` на старте), конфигурация (JSON под протокол; для `simulation` — паттерны по Tag'ам).
- **Создание и правка Environment**: имя (уникально в проекте), комментарий.
- **Привязка Device к Environment**: параметры связи (адрес, порт, учётные данные) — JSON под протокол.
- **Привязка Tag'а к Device**: у Tag'а поле «поставщик» (одно устройство или никто); два устройства на один Tag — ошибка (ADR 0004, amendment 2026-11-01).
- **Токены**: выпуск (секрет показывается **один раз**), отзыв, ротация, список с метаданными; «осиротевшие» токены (Device, которой больше нет) — видны.
- Всё это — **в режиме Project** (черновик), с правилами `uiutil` (версии, `Versions`, загрузка списков) и с реакцией на события `device_*`/`environment_*` по проектному SSE-потоку.

## Что уже готово в API

`GET/POST /projects/{id}/devices`, `GET/PUT/DELETE …/devices/{device_id}`, `GET/POST …/environments`, `GET/PUT/DELETE …/environments/{environment_id}`, `GET …/environments/{environment_id}/devices`, `PUT/DELETE …/devices/{device_id}`, `POST/DELETE …/devices/{device_id}/token`, `POST …/token/rotate`, `GET …/token`.

## Связанное

- Задача 34, часть 4 — REST.
- Задача 65 — отладка мнемосхем в Project-режиме на живых данных: тоже про режим Project и про значения Tag'ов; решать вместе, чтобы не переделывать панель Tag'ов.
- `internal/server/interface/ui/project/` — 28 файлов, правила режима: `uiutil.Call`, `uiutil.Load`, `uiutil.Versions`, `uiutil.FindByID`.

## Comments

- 2026-11-01 (grilling задачи 34): выделено из задачи 34 вместе с growctl-манифестом; спецификация — за инженером.
- 2026-11-07: после 72_6 Project-режим без EditLock, с Save/Commit; панель устройств делать по тем же правилам (Version Draft, 409 draft_conflict), без строки замка. Выполнять после 72_6.
