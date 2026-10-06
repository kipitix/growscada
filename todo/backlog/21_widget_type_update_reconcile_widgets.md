# Изменение WidgetType и согласование Widget — одна операция
Status: needs-triage
Blocked by: 20

Появилось в архитектурном ревью 2026-10-05. `UpdateWidgetType` (`application/widget_type_application_service.go`) изменяет чужой агрегат неатомарно и без событий.

## Что не так сейчас

- Порядок: `Save(type)` с commit → `WidgetTypeUpdatedEvent` → `removeOrphanedPortBindings`: `FindByWidgetTypeID` → для каждой Scene `ReconcileWith(wt)` → свой `sceneRepository.Save` в отдельной транзакции, повышающий Version Scene. При `ErrSceneConflict` Scene перечитывается и согласуется заново (до 3 попыток), удалённая тем временем Scene пропускается (задача 19).
- Ошибка посреди цикла (не конфликт, например отказ БД): клиент получает 500, но WidgetType уже сохранён, а часть Scene уже согласована; остальные Scene сохраняют привязки к исчезнувшим портам.
- `WidgetUpdatedEvent` и `SceneUpdatedEvent` не публикуются. `eventlog.ReloadTargetsFor("widget_type_updated")` перечитывает только WidgetTypes, у Project остаются устаревшие Version Scene, и следующая правка Widget получает 409.

## Что сделать

- Сохранение WidgetType и согласование всех затронутых Scene — в одной единице работы (задача 20): всё или ничего.
- После commit публикуются `widget_type_updated` и события изменённых Scene, чтобы UI получил новые Version.

## Приёмка

- Тест частичного отказа: при ошибке сохранения одной Scene не меняется ни WidgetType, ни одна Scene.
- Тест: после удаления InputPort в событиях есть каждая затронутая Scene.
- В браузере: удалить InputPort в Library, затем сразу передвинуть Widget этой Scene в Project — 409 нет.
- `make build` и `make test` проходят.
