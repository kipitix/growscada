# Типизированный клиент сервера для PWA UI
Status: needs-triage

Появилось в архитектурном ревью 2026-10-05. UI ходит в REST API напрямую из каждой операции и держит свои копии JSON-структур. Пересекается с задачей 29: там общий загрузчик списков, здесь — запись и типы.

## Что не так сейчас

- Прямые `http.*` вызовы: `ui/project/scene_ops.go` (8), `widget_ops.go` (5), `tags_ops.go` (3), `ui/library/widget_type_ops.go` (7); `uiutil.FetchJSON` умеет только GET. Блок `ctx.Dispatch(… toast.NetworkError(err))` повторён 21 раз.
- `widget_ops.go:createWidget` — около 75 строк: склейка URL, `json.Marshal`, `http.Post`, проверка статуса, декодирование, 10 присваиваний полей редактора.
- Учёт Version Scene (`currentSceneVersion`, `applyNewSceneVersion`) вручную протянут через каждую операцию.
- `ui/project/dto.go` (19 структур) и `ui/livescene/dto.go` (8 структур) вручную копируют `contract/api/v0`. Новое поле Widget приходится менять примерно в 10 местах.
- `internal/apiclient` уже разбирает Problem Details, но умеет только Tags, и UI его не использует.
- Операции UI не покрыты тестами, покрытие UI-пакетов около 1%.

## Что сделать

- Расширить `internal/apiclient` на Scenes, Widgets и WidgetTypes, на типах `contract/api/v0`.
- UI вызывает клиент вместо прямого HTTP. Ошибки клиента → toast в одном месте.
- Удалить UI-копии DTO в пользу `apiv0`.
- Шов с двумя адаптерами: HTTP и in-memory, чтобы операции Project и Library тестировались без сервера.
- Сначала проверить: работает ли apiclient под `GOOS=js GOARCH=wasm` и насколько растёт `app.wasm`. Заголовок `GrowSCADA-Schema-Version` (`uiutil/schema_version.go`, ADR 0005) сохранить.

## Приёмка

- Поведение UI не меняется. В браузере: создание, изменение, удаление Scene, Widget, Tag и WidgetType работает, ошибки показываются toast'ом.
- Есть тесты операций Project и Library на in-memory адаптере.
- `make build` и `make test` проходят.
