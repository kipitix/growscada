# Изменение WidgetType и согласование Widget — одна операция
Status: needs-triage
Blocked by: 19, 20

Появилось в архитектурном ревью 2026-10-05. `UpdateWidgetType` (`application/widget_type_application_service.go`) изменяет чужой агрегат неатомарно и без событий.

## Что не так сейчас

- Порядок: `Save(type)` с commit → `WidgetTypeUpdatedEvent` → `removeOrphanedPortBindings`, в котором на каждый Widget свой `sceneRepository.UpdateWidget` в отдельной транзакции, повышающий Version Scene.
- Ошибка посреди цикла: клиент получает ошибку, но WidgetType уже сохранён, а часть Widget уже переписана.
- `WidgetUpdatedEvent` и `SceneUpdatedEvent` не публикуются. `eventlog.ReloadTargetsFor("widget_type_updated")` перечитывает только WidgetTypes, у Project остаются устаревшие Version Scene, и следующая правка Widget получает 409.
- Сервис WidgetType собирает Widget по полям (`widget.NewWidget(w.ID(), w.Name(), w.Position(), …)`), то есть знание о конструкции Widget вытекает из домена Scene.

## Что сделать

- Правило «PortBinding ссылается только на существующие InputPort своего WidgetType» — поведение агрегата Scene (задача 19).
- Сохранение WidgetType и согласование всех затронутых Scene — в одной единице работы (задача 20): всё или ничего.
- После commit публикуются `widget_type_updated` и события изменённых Scene, чтобы UI получил новые Version.

## Приёмка

- Тест частичного отказа: при ошибке сохранения одной Scene не меняется ни WidgetType, ни одна Scene.
- Тест: после удаления InputPort в событиях есть каждая затронутая Scene.
- В браузере: удалить InputPort в Library, затем сразу передвинуть Widget этой Scene в Project — 409 нет.
- `make build` и `make test` проходят.
