# Detail у 400 для invalid input без внутренних обёрток
Status: needs-triage

Появилось при разборе задачи 22. Ответ 400 на `application.ErrInvalidInput` отдаёт полный `err.Error()`, например «cannot create scene: name: …» или «cannot save scene: type_id: widget type not found». Префикс поля клиенту полезен, но сервис добавляет его то снаружи `invalidInput()` (`"type_id: %w"`), то внутри, а вокруг стоят обёртки сервиса («cannot create scene:»). Поэтому без правки application вырезать его чисто не получится.

- Сделать так, чтобы сервис отмечал invalid input с полем и сообщением, например публичным типом ошибки с `Field`. Тогда restapi строит `detail` «field: сообщение» без обёрток сервиса.
- Решить, нужны ли полевые ошибки в расширении Problem Details (например, `errors[]`). Если да, это изменение контракта `contract/api/v0`: поднять SchemaVersion и выполнить `make schemas`.
- Поправить ожидания `detail` в тестах обработчиков. Проверить, как `apiclient` показывает `detail` (growctl, симулятор).
- Коды ответов не меняются.
