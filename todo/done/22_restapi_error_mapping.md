# Перевод ошибок в Problem Details — один модуль в restapi
Status: ready-for-agent

Появилось в архитектурном ревью 2026-10-05. Каждый обработчик REST сам знает доменные sentinel-ошибки и сам строит ответ, а `NewNotFound` вызывается неверно.

## Что не так сейчас

- 17 повторов `uuid.Parse(r.PathValue(...))` + `NewBadRequest` в `restapi/{widgets,scenes,tags,widget_types}.go`. Только в `widgets.go` разбор двух ID повторён 4 раза, а лестница `errors.Is(ErrSceneNotFound / ErrWidgetNotFound / ErrSceneConflict)` — 5 раз.
- Доменные sentinel-ошибки импортируются в restapi напрямую. `sendServiceError` (`router.go`) знает только `ErrInvalidInput`.
- Баг: все 17 вызовов `NewNotFound(err.Error(), idStr, r.URL.Path)` передают текст ошибки вместо типа ресурса. Клиент видит, например, «error on find scene by id in repository: scene not found with ID '…' not found», то есть внутренний текст обёрток уходит наружу.

## Что сделать

- Один модуль сопоставления «ошибка → Problem Details»: статус, тип ресурса, ID из пути. Неизвестная ошибка → 500 без внутреннего текста.
- Хелпер разбора UUID из пути с готовым 400.
- Обработчики используют только эти два хелпера, своих лестниц `errors.Is` в них нет.
- Табличный тест сопоставления без Postgres; поправить ожидания в тестах обработчиков на новый `detail`.

## Приёмка

- `detail` у 404 имеет вид «Scene with ID '…' not found», без текста внутренних обёрток.
- Коды ответов не меняются. Если меняются формулировки, это только `detail`, поэтому SchemaVersion поднимать не нужно. Bruno-коллекцию проверить.
- `make build` и `make test` проходят.

## Comments

### Решения (grilling 2026-10-13)

- **Модуль.** `restapi/errors.go`: таблица `{sentinel, статус, ресурс, detail для 409}` и одна функция `sendError(w, r, err)`. Доменные sentinel-ошибки импортирует только этот файл. Слой application не меняется.
- **Порядок.** Сначала `application.ErrInvalidInput` → 400 с `err.Error()` (как сейчас), затем строки таблицы, затем 500 (полная ошибка в лог, наружу общий текст). Правило: ссылка из тела запроса на несуществующее → 400, 404 только для ресурса из пути. Пример: `POST /scenes/{sceneId}/widgets` с несуществующим `type_id` остаётся 400, хотя ошибка совпадает и с `library.ErrWidgetTypeNotFound`.
- **404.** `detail` вида «Scene with ID '…' not found». ID берётся из `r.PathValue` по параметру, закреплённому за ресурсом. Для этого параметры шаблонов в `router.go` переименовать в `{tagId}`, `{widgetTypeId}`, `{sceneId}`, `{widgetId}`. Для клиента URL не меняются. Если строка 404 сработала, а параметра ресурса в пути нет (ошибка сервиса), отдаётся 500 с записью в лог.
- **409.** У каждой строки фиксированная нейтральная фраза, внутренний текст наружу не уходит. Для конфликта версий: «Scene was changed concurrently; reload it and retry», так же для WidgetType и Tag. Фраза верна и для CAS по версии клиента, и для исчерпанного `retryOnRace`, где клиент версию не присылал. По глоссарию проигранная гонка — не edit conflict. Отдельные фразы у `ErrWidgetTypeInUse` («WidgetType is used by Widgets») и у `ErrTagNameTaken` («Tag name is already taken»).
- **Имена ресурсов** из глоссария: Tag, WidgetType, Scene, Widget. `Title` у 409 — «Conflict: WidgetType».
- **Хелпер UUID из пути** отдаёт готовый 400 вида «sceneId: 'abc' is not a valid UUID».
- **Остаются в обработчиках** (это не сопоставление ошибок): `getTagsByName` при `ErrTagNotFound` → 200 с пустым списком; `GetTags` → 400, если указано больше одного из `name`, `name_pattern`, `name_regex`.
- **Табличный тест** покрывает все строки таблицы, invalid input поверх `ErrWidgetTypeNotFound` (→ 400), 404 без параметра в пути (→ 500) и неизвестную ошибку (→ 500 без внутреннего текста).
- **Контракт.** Меняются только `detail` и `title`, SchemaVersion не поднимается. В Bruno `detail` не проверяется, URL те же, правок не нужно.
- **Вне задачи.** `detail` у 400 для invalid input остаётся `err.Error()` с обёртками. Чистка вынесена в задачу 59.
