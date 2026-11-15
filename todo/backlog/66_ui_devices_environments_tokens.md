# UI: устройства, окружения и токены в режиме Project
Status: needs-triage
Priority: 3

Слой E. Термины — `GLOSSARY.md` (Device, Environment, Environment Device, Device Token).

Выделено из задачи 34 (части A) 2026-11-01: REST для устройств, окружений и токенов готов (часть 4), а интерфейса нет. **Спецификация не готова** — проектировать вместе с инженером.

## Что нужно в интерфейсе

- **Создание и правка Device**: имя (уникально в проекте), комментарий, протокол (`simulation` на старте), конфигурация (JSON под протокол; для `simulation` — паттерны по Tag'ам; типизируется задачей 69).
- **Создание и правка Environment**: имя (уникально в проекте), комментарий.
- **Привязка Device к Environment**: параметры связи (адрес, порт, учётные данные) — JSON под протокол.
- **Привязка Tag'а к Device**: у Tag'а поле «поставщик» (одно устройство или никто); два устройства на один Tag — ошибка (ADR 0004, amendment 2026-11-01). **REST этого поля пока не умеет**: поставщик есть в домене (`tag.Supplier`) и в ProjectFile (`supplier_device_id`), но ни `CreateTagRequest`, ни отдельного маршрута для него нет — привязку вводит задача 68, этот пункт панели ждёт её; остальная панель (устройства, окружения, токены) от неё не зависит.
- **Токены**: выпуск (секрет показывается **один раз**), отзыв, ротация, список с метаданными; «осиротевшие» токены (Device, которой больше нет) — видны.
- Всё это — **в режиме Project** (черновик), с правилами `uiutil` (версии, `Versions`, загрузка списков) и с реакцией на события `device_*`/`environment_*` по проектному SSE-потоку. Сейчас `eventlog.ReloadTargetsFor` отдаёт для них пустой набор с комментарием «The Project mode shows no Devices yet (task 66)» — таблицу нужно расширить.

## Что уже готово в API

`GET/POST /projects/{id}/devices`, `GET/PUT/DELETE …/devices/{device_id}`, `GET/POST …/environments`, `GET/PUT/DELETE …/environments/{environment_id}`, `GET …/environments/{environment_id}/devices`, `PUT/DELETE …/environments/{environment_id}/devices/{device_id}`, `GET/POST/DELETE …/devices/{device_id}/token`, `POST …/devices/{device_id}/token/rotate`.

## Связанное

- Задача 34, часть 4 — REST.
- Задача 68 — смена поставщика Tag'а и REST-поле привязки; без неё пункт «Привязка Tag'а к Device» не сделать.
- Задача 69 — типизированный конфиг `simulation`: панель правки конфигурации должна опираться на него, а не на сырой JSON.
- Задача 65 — отладка мнемосхем в Project-режиме на живых данных: тоже про режим Project и про значения Tag'ов; решать вместе, чтобы не переделывать панель Tag'ов.
- `internal/server/interface/ui/project/` — правила режима: `uiutil.Call`, `uiutil.Load`, `uiutil.Versions`, `uiutil.FindByID`.

## Comments

- 2026-11-01 (grilling задачи 34): выделено из задачи 34 вместе с growctl-манифестом; спецификация — за инженером.
- 2026-11-07: после 72_6 Project-режим без EditLock, с Save (saved state); панель устройств делать по тем же правилам (Version Draft, 409 draft_conflict), без строки замка. Выполнять после 72_6.
- 2026-11-15: сверено с кодом. Список маршрутов исправлен (`PUT/DELETE …/environments/{id}/devices/{device_id}`, а не `…/devices/{device_id}`); циклическая зависимость с 68 снята: панель идёт без `Blocked by`, а её пункт «привязка Tag'а» ждёт REST-поля из задачи 68. Зависимости 34_4 и 72_6 закрыты.
