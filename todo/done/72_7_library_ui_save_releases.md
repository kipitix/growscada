# Library UI: Save, история сохранений и Release из Commit

Status: ready-for-agent
Priority: 1
Blocked by: 72_5, 72_1

Слой E. Термины — `GLOSSARY.md` (Library, WidgetType, Commit, Library Release, Library Pin). Часть 7 из `72_save_release_deploy.md`.

## Задача

Library-режим переходит на Save → Commit → Release. Строка EditLock и Check in/out уже удалены в 72_1. Release делается из сохранения, а не из текущего Draft; Project видит Release в списке закреплений. Discard отменяет несохранённое — возвращает Draft к последнему Commit'у.

## Шапка Library

- **Save** — как в Project: заголовок (по умолчанию «Сохранение DD.MM.YYYY HH:MM»), автор (`author:name`), индикатор несохранённых изменений (Version Draft > `draft_version` последнего Commit). Save выключен без изменений; `Ctrl+S`/`Cmd+S` делает то же.
- **История сохранений** — Commit'ы Library: номер, заголовок, автор, время, опубликовано/нет, счётчик WidgetTypes; переименование заголовка; скачивание/просмотр содержимого (WidgetTypes) только для чтения.
- **Releases** — список Library Release: номер, заголовок Commit'а (или `legacy`), заметка, автор, время, счётчик WidgetTypes; просмотр содержимого Release только для чтения; заметка редактируется отдельной кнопкой. Кнопки Deploy нет.
- **Сделать Release** — у Commit'а в истории: заметка (необязательна), автор; `POST …/libraries/{id}/releases` `{commit, note, author}`.
- **Discard** — к последнему Commit'у (отменить несохранённое); без Commit'ов — 409 `nothing_to_discard`. Подтверждение: «Отменить изменения с последнего сохранения?». Одна транзакция, чистит undo/redo; Commit/Release не трогает.
- **Revert to latest Release** — отдельная кнопка: сброс Draft к последней опубликованной Release; без Release — 409. Не путать с Discard: Revert возвращает к опубликованному, а не к последнему Save.
- **Release state** в списке Library: «есть несохранённые» (Draft отличается от последнего Commit'а), «есть неопубликованные» (последний Commit новее последней Release), иначе «опубликовано».
- Без EditLock: редактор WidgetType доступен всегда; чужие правки дают 409 `draft_conflict` и перечитывание Draft/стека.

## Редактор WidgetType

- Правка WidgetType по-прежнему сохраняется сразу (DraftChange); Save фиксирует Commit.
- Release из Commit копирует WidgetTypes в `released_widget_types`; Project, закрепивший эту Release, видит новые WidgetTypes после смены закрепления (как сегодня; правка Draft Library не меняет Scene Project'а).
- «Есть неопубликованные изменения» — подсказка инженеру, что последний Save ещё не стал Release; Release не обязателен для правки, но нужен, чтобы Project увидел изменения.
- Один Commit — одна Release: у опубликованного Commit'а кнопка выключена; для новой Release нужен новый Save (если Draft не менялся, Save ответит `nothing_to_save` — тогда опубликовать нечего; это ожидаемо).

## Project: закрепления и видимость Library Release

- В панели Library Pins Project-режима показывать провенанс закреплённой Release: номер, заметку, автора, номер/заголовок Commit'а, `legacy` для старых. Список доступных Release — из `GET …/libraries/{id}/releases` (без содержимого, содержимое — по открытию).
- Смена/снятие закрепления — как сегодня: preview `impact` (сироты/сломанные PortBindings), Version Draft, без EditLock. После смены — обычные события.
- Палитра WidgetTypes Project'а — из закреплённых Release, без изменений.

## Удаление и защита

- Удаление Library с историей Commit'ов/Release блокируется 409 (задача 71/72_5); UI показывает причину и не предлагает «удалить насильно».
- Удаление Library без истории и без закреплений — как сегодня.
- Удаление Project не трогает Library и его Release; Commit'ы/Release Project'а уходят с Project (CASCADE).

## События и перезагрузки

- `commit_created`, `commit_title_updated`, `library_released` — перечитывать историю сохранений и Release выбранной Library; события другой Library не трогают выбранную (см. 63).
- `widget_type_*` — перечитывать Draft/список WidgetTypes и признак неопубликованных изменений; не перечитывать все Release с содержимым.
- `EventResync` — перечитать шапку, историю, Release state.
- 409 `draft_conflict` — перечитать Draft, стек, историю сохранений.

## Удалить из UI

- Строку замка, Check out/Check in/Take over, read-only без замка, `editlock:*`, ожидание Check in перед Release/Discard.
- Кнопку Release из текущего Draft (заменить на Release из Commit).
- Старый признак «невыпущенные изменения» на основе `DraftDiffers`, если он пересчитывается иначе; новый — по Commit/Release.

## Тесты

- `uitest`: Save Library, история, переименование, Release из Commit'а, повторная публикация — toast, Discard к Commit'у, Revert to latest Release, legacy Release, Project-пины с провенансом, 409 `draft_conflict`.
- Событие другой Library не перезагружает выбранную; `widget_type_updated` не тянет все Release с содержимым.
- Ручная проверка chrome-devtools: два таба, Save/Release, Project закрепляет Release и видит WidgetType.
- `make build`, `make test`, `make bruno_test`.

## Документы

- `AGENTS.md`: Library-режим (Save, Commit, Release, Discard к Commit'у, без EditLock; Release в Project-пинах).
- `CHANGELOG.md`.

## Вне объёма

- Публичность Library и права — задача 50.
- Retention Commit'ов.
- Полный редизайн редактора WidgetType.

## Comments

- 2026-11-07: выделено из 72. Решение пользователя: Discard в обоих режимах — к последнему Commit'у; Revert к опубликованному (Deployment/Release) — отдельное действие.
