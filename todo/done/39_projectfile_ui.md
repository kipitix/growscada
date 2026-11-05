# ProjectFile в UI: скачивание, открытие и Deploy из файла

Status: ready-for-agent
Blocked by: 35, 38

Слой C. ProjectFile уже определён (`contract/project/v0`: `Header` со `schema_version` и `project_id`, теги, устройства, окружения с парами, сцены с виджетами, копии WidgetTypes, пины), `ProjectFileBuilder` собирает его из Draft, `RuntimeNode.Deploy` принимает готовый документ, а `GET …/revisions/{n}` уже отдаёт `content` как `ProjectFile`. Не хватает четырёх действий: скачать Draft, скачать Revision, открыть файл в Draft, сделать из файла Revision.

## Маршруты

Все — под проектом (`/api/v0/projects/{project_id}`), формат тела и ответа файловых маршрутов — `projectv0.ProjectFile` как есть.

- `GET …/projectfile` — Draft как ProjectFile. Собирает `ProjectFileBuilder` по живому Project (Engineering). Читать можно без EditLock. Если Draft не собирается (Widget ссылается на тип вне пинов, Tag от чужого Device и т.п.) — `409 draft_not_deployable` с причиной в `detail`: неконсистентный Draft не отдаётся как файл.
- `GET …/revisions/{n}/projectfile` — Revision `n` как ProjectFile, ровно сохранённый документ (Runtime, `RevisionService.Find`). `404` для номера, которого у Project нет; Project не live — `404`.
- `POST …/projectfile?version=V` — открыть файл в Draft. `V` — Version Draft'а, которую видел клиент (`?version=`, как у DELETE: тело занято документом). Требует EditLock (`X-GrowSCADA-Client`) и проходит тот же гейт, что прочие записи Draft (ADR 0015, ADR 0016). Ответ `200 {version}` — Version после открытия. Файл, канонизированный вид которого равен текущему Draft, — `409 nothing_to_open`.
- `POST …/projectfile/deploy` — Deploy из файла: Revision из документа без Draft и без EditLock. Ответ `201 {number, deployed_at}`, как у обычного Deploy. Документ, равный текущей Revision, — `409 nothing_to_deploy`.

Скачиваемые файлы: `Content-Type: application/json`, `Content-Disposition: attachment; filename="<project>-draft.json"` / `"<project>-revision-<n>.json"`; имя Project санитизируется до безопасных символов, при пустом результате — ID. Внутри файла — документ как есть, без обёртки; `GrowSCADA-Schema-Version` остаётся заголовком API.

## Проверка файла

Одна проверка для открытия и для Deploy из файла, на Engineering-узле (ей нужны домены Engineering: `scene`, `device`, `tag`, `library`; на Runtime уходит только готовый `projectv0` через порт).

- Сначала — `schema_version`: тот же MAJOR, MINOR не новее серверного (`projectv0.SchemaVersion()`). Новее — `400` с сообщением «файл новее сервера» (понятнее, чем «неизвестное поле»); старее или равен — принимается. Затем строгий decode: неизвестные поля — `400` (ADR 0005: принимающая сторону отвергает незнакомое).
- `header.project_id` обязан совпадать с Project из пути; иначе `400` (ADR 0006: идентичность Project глобальна и хранится в файле). Подмены `project_id` нет.
- Доменные инварианты и ссылочная целостность: допустимые имена/типы/протоколы/размеры/языки скриптов, уникальные ID и естественные ключи (имена тегов и сцен в Project, имена типов в Library), `supplier_device_id` называет Device из файла, `PortBinding` — Tag из файла, пара `(environment, device)` ссылается на элементы файла, каждый `Widget.TypeID` назван в `widget_types`, каждый виджет принадлежит ровно одной сцене.
- Итог: пересобрать ProjectFile из будущего содержимого (те же правила канонизации, что у `ProjectFileBuilder`: списки по ID, ключи JSON отсортированы) и сравнить с канонизированным входным документом. Равенство значит, что Draft после открытия в точности равен файлу и файл задеплоится; расхождение (лишняя копия WidgetType, недостающий/лишний элемент) — `400`.
- Ошибки — `400` `invalid_input` с новым полем `Problem.file_issues: [{path, message}]`, где `path` — путь в документе (`schema_version`, `library_pins[0]`, `scenes[2].widgets[0].type_id`), и перечислены все найденные проблемы, а не первая.

`POST …/projectfile` дополнительно сверяет пины с локальными Libraries: для каждого `library_pins[i]` Library должна быть live и иметь Release с этим номером, а каждая копия `widget_types[i]` — совпадать (ID и содержимое) с `ReleasedWidgetType` этого Release. Иначе — `400 file_issues` с конкретным пином. Перепроверка идёт и внутри транзакции Draft (пины вставляются с `FOR SHARE` на live-строку Library, как `PutLibraryPin`), чтобы удаление Library не оставило пин на удалённую.

`POST …/projectfile/deploy` локальные Libraries не проверяет: документ самодостаточен (копии WidgetTypes внутри), Runtime node Libraries не читает. Поэтому файл с другого сервера запускается; цена — Draft после такого Deploy может ссылаться на отсутствующие здесь Releases (см. Discard).

## Открытие в Draft

- Ровно один DraftChange: `draft.Repository.Update` с `Session.Replace` (Purge + создание сущностей с их ID из файла), описание — `Open a project file`. Version Draft'а +1, redo-хвост отбрасывается как у любой новой правки, стек не очищается. В снапшот DraftChange попадают только фактически изменённые сущности (сравнение по ID и состоянию), поэтому объём записи пропорционален различию, а не двум Draft'ам.
- События — обычные события сущностей (`tag_created`/`tag_deleted`, `scene_*`, `widget_*`, `device_*`, `environment_*`, `library_pin_*`) в outbox той же транзакции, поэтому SSE и все режимы UI перезагружаются как при ручной правке, а undo восстанавливает прежний Draft своими inverse-событиями (ADR 0008, ADR 0017).
- Открытие — одна транзакция агрегата Draft (ADR 0016); никакой политики не нужно.
- Tag states и Device Tokens не трогаются: они Runtime-узла и меняются только следующим Deploy (ADR 0011).

## Deploy из файла

- Валидированный документ уходит на Runtime через порт `RuntimeNode` (новый метод, например `DeployFile`; in-process реализация — новый метод `RevisionService`, который передаёт cause `file` в `CreateRevision`). Runtime node в одной транзакции нумерует Revision, пишет документ с cause `file`, пересобирает производные копии тем же diff, что обычный Deploy, и публикует `revision_created`. Draft и его стек не трогаются, EditLock не нужен (действие как Rollback: не пишет Draft).
- Cause `file` — новое значение `RevisionCause` (`contract/api/v0` и `application/runtime`) и новое значение CHECK в `runtime.revisions`; `rolled_back_from` для `file` пуст. Миграция runtime: расширить `ck_runtime_revisions_cause` до `deploy | rollback | file` и `ck_runtime_revisions_rolled_back_from` (NULL для `deploy` и `file`).
- В истории Revision отображается как «Deployed from file»; обычный Deploy Draft'а не меняется и cause `deploy` сохраняет.

## Discard и чужие пины

Discard восстанавливает Draft из Revision. Если Revision называет Library Release, которого локально нет (следствие Deploy файла с другого сервера), Discard отвечает `409 revision_not_discardable` с `file_issues` по отсутствующим пинам и Draft не меняет — вместо падения на внешнем ключе. Если все пины локальны, Discard работает как раньше.

## Контракты и миграции

- `contract/api/v0`: `Problem.file_issues` + тип `FileIssue{path, message}`; `RevisionCauseFile = "file"`. `SchemaVersion` — MINOR (сейчас 0.18). `make schemas`.
- `contract/project/v0`: без изменений (валидация не добавляет полей).
- Миграция runtime — расширение CHECK'ов cause.
- `internal/apiclient`: методы скачивания (сырые байты + имя из `Content-Disposition`), открытия (`OpenProjectFile(version, data)`) и Deploy из файла; ошибки `file_issues` разбираются в `apiv0.Problem`.
- Bruno: по запросу на каждый маршрут (успех и отказ), в `seq`-порядке; тело `POST` — валидный файл, ошибки — `invalid-body`/отдельные запросы; `requestBodies` в тесте статической проверки.

## UI (Project mode)

- В шапке — одна кнопка «Project file…» (рядом с Revisions/Deploy/Discard). Её диалог:
  - `Download Draft` — GET `…/projectfile`, сохранение файла (Blob + `<a download>`), ошибка `draft_not_deployable` — тост.
  - `Open a file…` — выбор файла (`<input type=file accept=".json,application/json">`), разбор и подтверждение: имя, `schema_version`, `project_id`, счётчики Tags/Scenes/Widgets/Devices/Environments, пометка «заменяет весь Draft, можно отменить». Без EditLock кнопка недоступна с подсказкой, как остальные записи.
  - `Deploy a file…` — выбор файла, подтверждение: «Operation переключится на новую Revision, Draft не изменится», текущая Revision. Доступно без EditLock.
- Ошибки `file_issues` показываются списком (путь + сообщение), а не одной строкой.
- В диалоге Revisions у каждой строки — `Download` (`…/revisions/{n}/projectfile`); причина `file` отображается «Deployed from file».
- После открытия — перечитать содержимое и стек DraftChange (`{version}` из ответа и события); после Deploy из файла — preview и список Revisions. Файл не переживает перезагрузку страницы: это разовое действие.

## Проверки

- Юнит-тесты валидатора на каждый отказ: версия новее/чужой MAJOR, чужой `project_id`, неизвестное поле, дубликаты ID и имён, висячие ссылки, лишняя/несовпадающая копия WidgetType, отсутствующий локальный Release, Draft, который после открытия не равен файлу.
- Тест репозитория: открытие — одна транзакция, один DraftChange, Version +1, undo возвращает прежний Draft, redo-хвост отброшен.
- REST-тесты: четыре маршрута, `409 nothing_to_open`/`nothing_to_deploy`, `409 draft_not_deployable`, `400 file_issues`, `409 revision_not_discardable`, заголовки скачивания.
- UI-тесты: диалог, подтверждение, ошибки списком, скачивание (по возможности в `TestEngine`).
- `make build`, `make test`, `make bruno_test`; браузерная проверка всех четырёх действий (правило 9).

## Решения

- Deploy из файла — отдельный путь, а не «открыть файл и задеплоить Draft»: Revision создаётся из документа, Draft и стек DraftChange не трогаются, EditLock не нужен; после него preview показывает «Unreleased changes», Discard доступен.
- Четыре явных маршрута: `GET …/projectfile`, `GET …/revisions/{n}/projectfile`, `POST …/projectfile`, `POST …/projectfile/deploy`.
- `project_id` файла должен совпадать с целевым Project; подмены нет.
- `schema_version`: тот же MAJOR, MINOR не новее серверного; строгий decode. project/v0 не меняется.
- Валидация — до равенства: будущий Draft пересобирается в ProjectFile и сравнивается с канонизированным файлом; невалидный или незадеплоиваемый файл не применяется.
- Ошибки — `400` с `Problem.file_issues` (список), контракт API — MINOR.
- Открытие требует EditLock и Version, это один DraftChange с diff-снапшотом; равный Draft — `409 nothing_to_open`.
- Открытие сверяет пины с локальными Releases; Deploy файла — нет (самодостаточный документ).
- Cause Revision из файла — `file`; миграция CHECK и API MINOR.
- Discard Revision с отсутствующими локально Releases — `409 revision_not_discardable`, Draft не меняется.
- Файлы: `<project>-draft.json`, `<project>-revision-<n>.json`; `application/json`, `Content-Disposition: attachment`.
- UI: один диалог «Project file…» в шапке + Download в строках Revisions; подтверждение с разбором файла.
- Задача остаётся одной, без деления на части.

## Comments

- 2026-10-04: бывший тикет 06 задачи 36. Решения: Q23–Q25 (два файла — ProjectFile и PlaybackFile), Q29 (открытый ProjectFile заменяет Draft одной DraftChange, требует EditLock). Стоит после DraftChange (задача 38), поэтому перенесён из слоя B в слой C.
- 2026-11-05: грилинг (вопросы Q1–Q19) и аудит DDD. Отдельный Deploy из файла без Draft; четыре маршрута; совпадение `project_id`; политика версии файла (тот же MAJOR, MINOR ≤ сервера); валидация до канонического равенства и её ошибки через `file_issues` (400); открытие — один DraftChange с diff-снапшотом, EditLock и Version, равный Draft — 409; пины сверяются при открытии и не сверяются при Deploy файла; cause `file`; Discard чужой Revision — 409 `revision_not_discardable`; имена файлов; UI-диалог; задача не делится. Термины вынесены в `GLOSSARY.md`, решения — в ADR 0018.
