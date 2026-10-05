# Перевод ошибок в Problem Details — один модуль в restapi
Status: needs-triage

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
