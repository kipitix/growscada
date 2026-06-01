# growscada [CHANGELOG](https://keepachangelog.com/en/1.1.0/)

## [0.0.18] - 2026-06-02

### Fixed

- `internal/interface/ui/project` — исправлено смещение виджета при изменении размера через хендл SE после поворота (`project.go`, `render_scene.go`):
  - **Причина**: CSS-матрица виджета содержит трансляцию `e = posX + ox·(1−cosA) + oy·sinA`, `f = posY − ox·sinA + oy·(1−cosA)`, где `ox = originX·W`, `oy = originY·H`. При изменении ширины/высоты `ox` и `oy` менялись, сдвигая левый-верхний угол виджета на экране — даже если `posX`/`posY` оставались прежними
  - При mousedown на хендле SE теперь фиксируется канвас-позиция левого-верхнего угла (`resizeStartE`, `resizeStartF`) и текущий `origin` (`resizeOriginX`, `resizeOriginY`)
  - В каждом mousemove после вычисления нового размера `posX`/`posY` пересчитываются так, чтобы `(e, f)` оставалась неизменной: `newPosX = E − newOx·(1−cosA) − sinA·newOy`, `newPosY = F + sinA·newOx − newOy·(1−cosA)`
  - Панель свойств обновляет поля X/Y синхронно с изменением размера

## [0.0.17] - 2026-06-01

### Added

- `internal/interface/ui/project` — панель **Properties** при отсутствии выбранного виджета теперь отображает свойства текущей сцены вместо текста «Select a widget» (`render_properties.go`, `scene_ops.go`, `project.go`):
  - Поля Name (text input), Size W × H (number spinbutton), Background HTML (textarea) — редактируемые, сохраняются через PUT `/api/v1/scenes/:id` при потере фокуса или нажатии Enter
  - Добавлены поля `editingScenePropsName`, `editingScenePropsWidth`, `editingScenePropsHeight`, `editingScenePropsBG` в структуру `Project`
  - `syncSceneEditingFields()` — синхронизирует поля панели с выбранной сценой; вызывается при загрузке, переключении вкладки сцены и переименовании через inline-редактор
  - `saveSceneProperties(ctx)` — отправляет PUT с оптимистичным обновлением in-memory + обновлением `Version` из ответа

### Fixed

- `internal/interface/ui/project` — исправлен сброс выбора виджета: один клик по холсту теперь снимает фокус (`project.go`, `render_scene.go`, `widget_ops.go`):
  - **Причина**: `OnMouseDown` на виджете сразу выставлял `draggingWidgetID`, из-за чего `finalizeAllDrags` при `mouseup` ставил `dragJustEnded = true` даже без реального перемещения — первый клик по сцене проглатывался
  - Добавлен флаг `dragDidMove bool`: выставляется в `OnMouseMove` только при наличии активного drag-операции; `finalizeAllDrags` теперь устанавливает `dragJustEnded = dragDidMove` (и сбрасывает `dragDidMove = false`) — подавление срабатывает только после реального перетаскивания, а не при обычном клике
  - Добавлен `OnMouseDown` на холст, сбрасывающий `dragJustEnded`: устраняет случай, когда после drag браузер не генерирует `click` (mousedown и mouseup на разных элементах) — флаг оставался `true` и следующий намеренный клик по сцене тоже проглатывался

## [0.0.16] - 2026-06-01

### Fixed

- `internal/interface/ui/project` — исправлен баг с перемещением точки **Origin** у повёрнутого виджета (`project.go`, `render_scene.go`):
  - При ненулевом угле поворота перетаскивание хендла Origin перемещало весь виджет вместо точки привязки. Три последовательно устранённые ошибки:
    1. Экранная дельта мыши не преобразовывалась в локальное пространство виджета; добавлено обратное вращение: `d_ox = cosA·dx + sinA·dy`, `d_oy = −sinA·dx + cosA·dy` с сохранением `originDragRotDeg` при начале drag
    2. Изменение `(ox, oy)` смещало компоненты трансляции CSS-матрицы (`e = posX + ox·(1−cosA) + oy·sinA`), визуально сдвигая виджет; добавлена компенсация позиции: `newPosX = startPosX − dOx·(1−cosA) − dOy·sinA`, `newPosY = startPosY − dOy·(1−cosA) + dOx·sinA`; добавлены поля `originStartPosX`, `originStartPosY`
    3. Два исправления объединены: дельта мыши сначала поворачивается в локальные оси виджета, затем кламп-скорректированные локальные дельты подставляются в формулу компенсации позиции — хендл точно следует за курсором при любом угле поворота, виджет остаётся на месте
- `internal/interface/ui/project` — исправлен сброс выбора (selection) виджета при отпускании хендлов вращения и Origin над холстом (`render_scene.go`, `widget_ops.go`, `project.go`):
  - После завершения drag браузер генерировал отдельное событие `click`, которое поднималось к canvas (минуя контейнер виджета) и вызывало `clearWidgetSelection`
  - Добавлен `OnClick` → `stopPropagation` на контейнер виджета: перехватывает клики по хендлам, когда мышь остаётся в пределах виджета
  - Добавлен флаг `dragJustEnded bool`: `finalizeAllDrags` выставляет его при завершении любого drag; canvas `OnClick` при виде флага сбрасывает его и пропускает очистку selection — покрывает случай отпускания мыши над canvas вне контейнера виджета

## [0.0.15] - 2026-05-29

### Added

- `internal/domain/widget` — **InputPort** и **PortBinding** Value Objects:
  - `InputPortName` — валидируется как JS-идентификатор (`^[a-zA-Z_$][a-zA-Z0-9_$]*$`); `NewInputPortNameFromStorage` пропускает regex для доверенных данных из БД
  - `InputPort` — именованный типизированный слот ввода на WidgetType; поля `Name`, `Description`, `TypeHint` (`tag.TagType`); `TypeHint == TagTypeUnknown` означает «любой тип тега»
  - `PortBinding` — связь порта (по имени) с конкретным экземпляром `Tag` (через `id.ID[tag.Tag]`)
  - `NewWidgetType` возвращает `(WidgetType, error)` и проверяет уникальность имён портов; при дубликате — `ErrDuplicateInputPortName`
  - `WidgetType.InputPorts()` — возвращает защитную копию среза
  - `Widget.PortBindings()` заменяет `Widget.TagIDs()` — виджет теперь хранит именованные привязки портов вместо плоского списка UUID тегов
- `internal/application` — `InputPortDTO`, `PortBindingDTO` добавлены в `appdto/widget.go` и `appdto/widget_type.go`; application-сервисы `WidgetService` и `WidgetTypeService` маппят порты и привязки через `NewInputPort` / `NewPortBinding`
- `internal/infrastructure/postgres/migrations`:
  - `20260601000000_add_input_ports_to_widget_types.sql` — колонка `input_ports JSONB NOT NULL DEFAULT '[]'` в `widget_types`; элемент: `{"name":"…","description":"…","type_hint":"…"}`
  - `20260601000001_replace_tag_ids_with_port_bindings_in_widgets.sql` — добавляет `port_bindings JSONB`, мигрирует существующие `tag_ids` в синтетические имена `port_0`, `port_1`, …, затем удаляет колонку `tag_ids`
- `internal/infrastructure/postgres/repositories` — репозитории `widget_repository_postgres` и `widget_type_repository_postgres` сериализуют/десериализуют `InputPort[]` и `PortBinding[]` как JSONB; `widget_type_repository_postgres_test.go` — новый набор интеграционных тестов
- `internal/interface/restapi/restdto` — `InputPortDTO` и `PortBindingDTO` добавлены в REST-DTO; поле `tag_ids []uuid` заменено на `port_bindings []PortBindingDTO` в `WidgetRequest/Response`; `input_ports []InputPortDTO` добавлено в `WidgetTypeRequest/Response`
- `internal/interface/ui/library` — редактор типов виджетов расширен колонкой **Input Ports**:
  - список существующих портов с кнопкой удаления (✕)
  - форма добавления: поля Name (JS identifier), Description (опционально), Type Hint (any / string / boolean / integer)
  - порты сохраняются через PUT вместе с остальными полями типа виджета
- `internal/interface/ui/project` — панель свойств виджета заменяет тег-список секцией **Port Bindings**:
  - для каждого входного порта WidgetType отображается `<select>` с тегами, отфильтрованными по `type_hint`; текущая привязка подсвечивается; можно сменить тег или снять привязку (`— unbound —`)
  - изменения сохраняются через PUT виджета
- `internal/interface/ui/uidto` — новый пакет `uidto` с `InputPortDTO`, общим для пакетов `library` и `project`, устраняет дублирование DTO
- `internal/interface/ui/root` — система тем оформления:
  - CSS custom properties (`var(--bg)`, `var(--accent)`, `var(--error)`, …) инжектируются через `<style id="gs-theme-vars">` в `<head>` при каждой смене темы
  - Три режима: `light`, `dark`, `auto` (авто использует `@media (prefers-color-scheme: dark)`)
  - Кнопка переключения темы: одиночный cycling-значок `◑` (auto) → `☀` (light) → `☾` (dark); режим сохраняется в `localStorage` (`root:theme`)
  - CSS-сброс `html, body { margin: 0; overflow: hidden }` устраняет постоянно видимый вертикальный scrollbar
- `internal/interface/ui/project` — изменяемые ширины панелей в Scenes:
  - Левая панель Widget Types и правая панель Properties разделены 5px-разделителями с `cursor: col-resize`
  - Drag-изменение ширины: левая [100, 480] px, правая [160, 600] px; ширины персистируются в `localStorage` (`project:widgetTypeWidth`, `project:propertiesWidth`)
- Диалоги подтверждения (`app.Window().Call("confirm", …).Bool()`) перед всеми деструктивными операциями в Library и Project
- Персистентность состояния UI в `localStorage`: активная вкладка навигации (`root:mode`), активная сцена (`project:sceneID`), активная под-вкладка Scenes/Tags (`project:subTab`), выбранный тип виджета в Library (`library:selectedID`), выбранный виджет на холсте
- Юнит-тесты `internal/domain/widget/input_port_test.go` — `TestNewInputPortName_*`, `TestNewInputPort_*`, `TestNewWidgetType_DuplicateInputPortName`, `TestNewWidgetType_UniqueInputPorts_OK`, `TestNewWidgetType_InputPortsCopied`

### Changed

- `Widget.TagIDs() []id.ID[tag.Tag]` → `Widget.PortBindings() []PortBinding` во всём стеке (домен → application → инфраструктура → REST → UI)
- `NewWidgetType` сигнатура: добавлен параметр `someInputPorts []InputPort`; возврат изменён с `WidgetType` на `(WidgetType, error)`
- `tag.NewTagType` принимает `"unknown"` и `""` как синонимы `TagTypeUnknown` без ошибки — упрощает маппинг `type_hint` из JSON
- `internal/interface/restapi/restdto/widget_type.go` — `WidgetTypeResponse.InputPorts` ранее отсутствовало и добавлено в Put-запрос; имя поля `input_ports` унифицировано с JSONB-схемой БД
- Все UI-компоненты Library и Project переведены с жёстко заданных hex-цветов на CSS custom properties; кнопки Delete стилизованы красным (`var(--error-*)`), Create — приглушённым акцентом (`var(--accent-*)`)

### Fixed

- Виджет не пропадал с холста после удаления: `loadWidgets` внутри `ctx.Dispatch` запускал `ctx.Async` в запрещённом контексте; заменено прямой фильтрацией среза `p.widgets` (оптимистичное обновление UI) — `widget_ops.go`
- Перетаскивание точки Origin перемещало виджет: удалена компенсация `Position.X/Y`; теперь изменяются только `Origin.X/Y` — `render_scene.go`, `project.go`
- `WidgetTypeService.UpdateWidgetType` при пустом срезе `InputPorts` в запросе обнулял порты вместо сохранения существующих; исправлено явной передачей портов из request — `widget_type_application_service.go`
- `WidgetService.UpdateWidget` аналогично игнорировал `PortBindings` при обновлении позиции/размера — `widget_application_service.go`, `widget_ops.go`
- UI Library: форма добавления порта не очищалась после добавления и не валидировала пустое имя — `render_editor.go`

## [0.0.14] - 2026-05-28

### Added

- `internal/interface/ui/project` — вкладка **Tags** в Project-компоненте:
  - `projectSubTab` — тип-перечисление (`scenes` / `tags`); `activeSubTab` хранит активную вкладку; `renderSubTabs()` рендерит таб-бар; `subTab()` — helper для отдельного таба с подсветкой активного состояния
  - `tags_render.go` — рендеринг тегов: `renderTagsPanel` (две колонки), `renderTagListColumn` (список с кнопками Create/Delete), `renderTagList` (кликабельные строки с именем и типом), `renderTagPropertiesPanel` (панель свойств выбранного тега: Name, Type, Value, Quality), `renderTagCreateForm` (форма с полями Name и Type, кнопки Create/Cancel)
  - `tags_ops.go` — сетевые операции: `createTag` (POST `/api/v1/tags`), `deleteTag` (DELETE `/api/v1/tags/{id}`); вспомогательная `defaultTagValue` возвращает начальное значение по типу тега
- `Makefile` — цель `full_restart`: `db_down` → `db_up` → `run`

### Changed

- `dto.go` — `tagItem` расширен полями `Type`, `Value`, `Quality`, `Version`; добавлены `createTagRequest` и `createTagResponse`
- `docs/ubiquitous_language/ubiquitous_language.md` — термин `Alarm` переименован в `Alert` во всех вхождениях

## [0.0.13] - 2026-05-27

### Fixed

- Оптимистичная блокировка виджетов: `UpdateWidget` с `version=0` в теле запроса тихо уходил в INSERT вместо UPDATE, вызывая ошибку PK-ограничения (HTTP 500 вместо 409). Исправлено: application-сервис выполняет `FindByID` + сравнение версий до `Save`; репозиторий добавил ветку `IsCommitted()` и fallback-ошибку для неопределённых состояний версии
- Оптимистичная блокировка сцен: `UpdateSceneInput` не содержал поле `Version`, что делало OCC структурно невозможным. Исправлено: добавлено поле `Version int` в `UpdateSceneInput`, `UpdateSceneRequest` и `UpdateSceneResponse`; `UpdateScene` проверяет `found.Version().Number() != input.Version` → `ErrSceneConflict`
- Оптимистичная блокировка типов виджетов: `UpdateWidgetType` игнорировал версию клиента — конкурентные правки перезаписывали друг друга без 409. Исправлено аналогичной проверкой версии в application-сервисе
- Nil-UUID валидация: `CreateWidget`/`UpdateWidget` с отсутствующим `scene_id` или `type_id` в JSON тихо сохраняли виджет с произвольным UUID. Исправлено явными проверками `== uuid.Nil` → `ErrWidgetInvalidInput` в application-сервисе
- `PostScenes` возвращал HTTP 500 при ошибках валидации домена; теперь возвращает 422 Unprocessable Entity при `ErrSceneValidation`
- `PostWidgets` возвращал HTTP 500 при невалидном вводе; теперь возвращает 400 Bad Request при `ErrWidgetInvalidInput`

### Changed

- `classifyUpdateConflict` — вынесена из четырёх репозиториев в единую пакетную generic-функцию `classifyUpdateConflict[T]` в `update_conflict_classification.go`; введён тип `tableName` с константами `tableNameTags`, `tableNameScenes`, `tableNameWidgets`, `tableNameWidgetTypes`, исключающий передачу произвольных строк
- `renderSceneCanvas` — удалён избыточный клиентский фильтр `w.SceneID != p.selectedSceneID`; `FindBySceneID` уже ограничивает выборку на уровне БД
- `finalizeAllDrags` — четыре одинаковых if-блока заменены циклом по `[]*string` с указателями на поля структуры
- Тесты: удалена функция-заглушка `mustTagIDFromUUID` (возвращала аргумент без изменений); удалена мёртвая переменная `_ = i` в range-цикле `widget_application_service_test.go`

## [0.0.12] - 2026-05-27

### Added

- `internal/domain/scene` — новый пакет домена для сцен:
  - `Scene` — агрегат (интерфейс + `sceneImpl`): `ID`, `Name`, `Size`, `BackgroundHTML`, `Version`
  - `SceneName` — Value Object; валидация: пустая строка возвращает ошибку
  - `SceneSize` — Value Object; ширина и высота в пикселях; `NewSceneSize` возвращает ошибку при неположительных значениях
  - `BackgroundHTML` — Value Object-обёртка над строкой статического HTML-фона сцены
  - `SceneRepository` — интерфейс с методами `NextID`, `Save`, `FindByID`, `FindAll`, `DeleteByID`; sentinel-ошибки `ErrSceneNotFound`, `ErrSceneConflict`, `ErrSceneValidation`
- `internal/domain/widget` — расширен агрегатом экземпляра виджета:
  - `Widget` — агрегат (интерфейс + `widgetImpl`): экземпляр `WidgetType`, размещённый на `Scene`; поля: `ID`, `Name`, `Position`, `Size`, `Origin`, `Rotation`, `TransformationMatrix`, `TypeID`, `SceneID`, `Labels`, `TagIDs`, `Version`
  - `WidgetName` — Value Object; валидация: пустая строка возвращает ошибку
  - `Position` — Value Object; координаты на холсте (`x`, `y` float64, `z` int для z-index)
  - `Size` — Value Object; ширина и высота в пикселях; `NewSize` возвращает ошибку при неположительных значениях
  - `Origin` — Value Object; нормализованная точка привязки трансформации (`0.0`–`1.0` по каждой оси)
  - `Rotation` — Value Object; угол поворота в градусах (float64)
  - `TransformationMatrix` — Value Object; 2D аффинная матрица CSS `matrix(a,b,c,d,e,f)`, вычисляется из `Position`, `Origin`, `Rotation`, `Size` с учётом точки привязки; метод `CSS()` возвращает строку для `transform:`
  - `WidgetRepository` — интерфейс с методами `NextID`, `Save`, `FindByID`, `FindAll`, `FindBySceneID`, `DeleteByID`; sentinel-ошибки `ErrWidgetNotFound`, `ErrWidgetConflict`, `ErrWidgetInvalidInput`
- `internal/domain/id` — новый пакет обобщённого Value Object `ID[T]`:
  - `ID[T]` — UUID-обёртка с типовым параметром агрегата; исключает перепутывание ID разных агрегатов на этапе компиляции
  - `NewID[T]`, `IDWithUUID[T]`, `IDOption[T]`; при создании без опций генерирует новый UUID автоматически
- `internal/domain/event` — события для жизненного цикла `Widget` и `Scene`:
  - `WidgetCreatedEvent`, `WidgetUpdatedEvent`, `WidgetDeletedEvent`
  - `SceneCreatedEvent`, `SceneUpdatedEvent`, `SceneDeletedEvent`
  - `EventType` расширен 6 новыми значениями
- `internal/application/widget_application_service.go` — `WidgetService` с методами `FindAllWidgets`, `FindWidgetsBySceneID`, `FindWidgetByID`, `CreateWidget`, `UpdateWidget`, `DeleteWidgetByID`; валидация обязательных полей `scene_id` и `type_id` на уровне сервиса; публикует события через `EventBus`
- `internal/application/scene_application_service.go` — `SceneService` с методами `FindAllScenes`, `FindSceneByID`, `CreateScene`, `UpdateScene`, `DeleteSceneByID`; публикует события через `EventBus`
- `internal/application/appdto/widget.go` — DTO `Widget`, `CreateWidgetInput`, `UpdateWidgetInput`; фабрики `NewWidget`, `NewWidgetList`
- `internal/application/appdto/scene.go` — DTO `Scene`, `CreateSceneInput`, `UpdateSceneInput`; фабрики `NewScene`, `NewSceneList`
- `internal/infrastructure/postgres/repositories/widget_repository_postgres.go` — PostgreSQL-реализация `WidgetRepository`; INSERT при `version == Initial`, UPDATE с оптимистичной блокировкой; `classifyUpdateConflict` определяет `ErrWidgetNotFound` vs `ErrWidgetConflict`
- `internal/infrastructure/postgres/repositories/scene_repository_postgres.go` — PostgreSQL-реализация `SceneRepository`; аналогичная стратегия оптимистичной блокировки
- Миграции:
  - `20260516000000_create_widgets.sql` — DDL таблицы `widgets`
  - `20260516000001_create_scenes.sql` — DDL таблицы `scenes`
  - `20260516000002_add_scene_id_to_widgets.sql` — добавлен FK `scene_id` в `widgets`
  - `20260518000001_add_size_to_widgets.sql` — добавлены колонки `width`, `height` в `widgets`
  - `20260526000000_add_transform_to_widgets.sql` — добавлены `origin_x`, `origin_y`, `rotation_degrees` в `widgets`
  - `20260527000000_widget_scene_id_not_null.sql` — `scene_id` переведён в `NOT NULL`
- `internal/interface/restapi/widgets.go` — REST-хэндлеры CRUD для экземпляров виджетов:
  - `GET /api/v1/widgets` — список всех виджетов
  - `GET /api/v1/widgets/{id}` — виджет по ID; 404 при отсутствии
  - `POST /api/v1/widgets` — создание; 201 с объектом; 400 при невалидном вводе
  - `PUT /api/v1/widgets/{id}` — обновление; 404 при отсутствии, 409 при конфликте версий
  - `DELETE /api/v1/widgets/{id}` — удаление; 200 с телом удалённого объекта, 404 при отсутствии
  - `GET /api/v1/scenes/{id}/widgets` — виджеты по сцене (вложенный ресурс)
- `internal/interface/restapi/scenes.go` — REST-хэндлеры CRUD для сцен:
  - `GET /api/v1/scenes` — список всех сцен
  - `GET /api/v1/scenes/{id}` — сцена по ID; 404 при отсутствии
  - `POST /api/v1/scenes` — создание; 201; 422 при ошибке валидации домена
  - `PUT /api/v1/scenes/{id}` — обновление; 404, 409 (конфликт версий), 422 (валидация)
  - `DELETE /api/v1/scenes/{id}` — удаление; 200 с телом удалённой сцены
- `internal/interface/restapi/restdto/widget.go` — HTTP-DTO `WidgetResponse`, `GetWidgetsResponse`, `CreateWidgetRequest/Response`, `UpdateWidgetRequest/Response`
- `internal/interface/restapi/restdto/scene.go` — HTTP-DTO `SceneResponse`, `GetScenesResponse`, `CreateSceneRequest/Response`, `UpdateSceneRequest/Response`
- Bruno-коллекция — новые запросы: `get_scenes`, `get_scene_by_id`, `post_scenes`, `put_scene_by_id`, `delete_scene_by_id`, `get_widgets`, `get_widget_by_id`, `get_widgets_by_scene_id`, `post_widgets`, `put_widget_by_id`, `delete_widget_by_id`
- Юнит-тесты домена `widget` — `widget_test.go`, `widget_name_test.go`, `widget_position_test.go`, `widget_size_test.go`, `widget_origin_test.go`, `widget_rotation_test.go`, `widget_transform_matrix_test.go`
- Юнит-тесты домена `scene` — `scene_test.go`, `scene_name_test.go`, `scene_size_test.go`, `scene_background_html_test.go`
- Юнит-тесты `internal/domain/id/id_test.go` — генерация нового UUID, задание через `IDWithUUID`, `String`, тип-параметр как изолятор
- Интеграционные тесты `internal/application/widget_application_service_test.go` — полный CRUD, валидация обязательных полей, конфликт версий, публикация событий
- Интеграционные тесты `internal/infrastructure/postgres/repositories/widget_repository_postgres_test.go`
- Интеграционные тесты `internal/interface/restapi/widgets_test.go` и `scenes_test.go` — все хэндлеры включая 409 Conflict и 422 Unprocessable Entity

### Changed

- `internal/domain/id` — `TagID`, `WidgetTypeID` и все прочие агрегатные идентификаторы переведены на обобщённый `id.ID[T]` из нового пакета `internal/domain/id`; per-aggregate `TagID`, `WidgetTypeID` удалены как отдельные типы
- `internal/domain/version` — `Version` стал обобщённым `Version[T]`; конструкторы `Initial[T]()` и `Committed[T]()` получили тип-параметр агрегата; исключает применение версии одного агрегата к другому
- `internal/interface/ui/library` — `Library` разбита на отдельные файлы: `library.go` (структура и монтирование), `dto.go`, `render_list.go`, `render_editor.go`, `widget_type_ops.go` (сетевые операции)
- `internal/interface/ui/project` — монолитный `project.go` разбит на: `project.go` (корневая структура), `dto.go`, `matrix.go`, `render_scene.go`, `render_properties.go`, `render_widget_types.go`, `scene_ops.go`, `widget_ops.go`
- Bruno-коллекция — все имена файлов с пробелами переименованы в snake_case (например, `get tag by id.yml` → `get_tag_by_id.yml`)

## [0.0.11] - 2026-05-16

### Added

- `internal/domain/widget` — новый пакет домена для типов виджетов:
  - `WidgetType` — агрегат (интерфейс + `widgetTypeImpl`); immutable: поля не мутируются напрямую, обновление через создание нового экземпляра в репозитории
 - `WidgetTypeID` — Value Object на основе UUID; функциональные опции `WidgetTypeIDWithUUID`; `ParseWidgetTypeID`, `MustParseWidgetTypeID`
  - `WidgetTypeName` — Value Object; валидация: пустая строка возвращает ошибку
  - `HtmlTemplate`, `Script` — Value Object-обёртки над строками; валидация зарезервирована (TODO)
  - `ScriptLanguage` — enum-VO (`javascript`, `python`, `lua`); сериализуется строкой; `NewScriptLanguage` возвращает ошибку для неизвестных значений
  - `WidgetTypeRepository` — интерфейс репозитория с методами `NextID`, `Save`, `FindByID`, `FindAll`, `DeleteByID`; sentinel-ошибки `ErrWidgetTypeNotFound` и `ErrWidgetTypeConflict` (optimistic locking)
- `internal/domain/version` — новый пакет Value Object `Version` для оптимистичной блокировки: константы `Initial` (0) и `Committed` (1); методы `Next()`, `IsCommitted()`, `Number()`; конструктор с валидацией на отрицательные числа
- `internal/domain/event` — события для жизненного цикла `WidgetType`: `WidgetTypeCreatedEvent`, `WidgetTypeUpdatedEvent`, `WidgetTypeDeletedEvent`; `EventType` расширен тремя новыми значениями
- `internal/application/widget_type_application_service.go` — `WidgetTypeService` с методами `FindAllWidgetTypes`, `FindWidgetTypeByID`, `CreateWidgetType`, `UpdateWidgetType`, `DeleteWidgetTypeByID`; публикует события через `EventBus` после каждой успешной мутации
- `internal/application/appdto/widget_type.go` — DTO `WidgetType`, `CreateWidgetTypeInput`, `UpdateWidgetTypeInput`; фабрики `NewWidgetType`, `NewWidgetTypeList`
- `internal/infrastructure/postgres/repositories/widget_type_repository_postgres.go` — PostgreSQL-реализация `WidgetTypeRepository`; INSERT при `version == Initial`, UPDATE с `WHERE id = $X AND version = $Y` при `IsCommitted()`; при `rowsAffected == 0` метод `classifyUpdateConflict` делает `SELECT EXISTS` и возвращает `ErrWidgetTypeNotFound` или `ErrWidgetTypeConflict`
- `internal/infrastructure/postgres/migrations/20260515000000_create_widget_types.sql` — DDL таблицы `widget_types`
- `internal/infrastructure/postgres/test_data/20260515000001_insert_test_data.sql` — тестовые данные для `widget_types`
- `internal/interface/restapi/widget_types.go` — REST-хэндлеры CRUD:
  - `GET /api/v1/widget-types` — список всех типов виджетов
  - `GET /api/v1/widget-types/{id}` — тип виджета по ID; 404 при отсутствии
  - `POST /api/v1/widget-types` — создание; 201 с ID
  - `PUT /api/v1/widget-types/{id}` — обновление; 404 при отсутствии, 409 при конфликте версий
  - `DELETE /api/v1/widget-types/{id}` — удаление; 200 с телом удалённого объекта, 404 при отсутствии
- `internal/interface/restapi/restdto/widget_type.go` — HTTP-DTO `WidgetTypeResponse`, `GetWidgetTypesResponse`, `CreateWidgetTypeRequest/Response`, `UpdateWidgetTypeRequest/Response`
- `internal/interface/ui/library/library.go` — UI-компонент `Library` для управления типами виджетов: список, создание, редактирование, удаление через REST API
- Bruno-коллекция: запросы `GET /widget-types`, `GET /widget-types/{{ID}}`, `POST /widget-types`, `PUT /widget-types/{{ID}}`, `DELETE /widget-types/{{ID}}`
- Юнит-тесты домена `widget` — `widget_type_test.go`, `widget_type_id_test.go`, `widget_type_name_test.go`, `widget_type_script_language_test.go`
- Юнит-тесты `internal/domain/version/version_test.go` — `New` с валидными значениями, отрицательное → ошибка, константы, `Next`, `IsCommitted`, `String`
- Интеграционные тесты `internal/application/widget_type_application_service_test.go` — CRUD, not-found, конфликт версий, публикация событий
- Интеграционные тесты `internal/interface/restapi/widget_types_test.go` — все CRUD-хэндлеры включая 409 Conflict

### Changed

- `internal/domain/version` — версия тега перенесена из `int` в `version.Version`; `Tag.Version()` теперь возвращает `version.Version`; `tag.TagVersionInitial`/`TagVersionCommitted` удалены
- `Tag.IncrementVersion()` — удалён из интерфейса и реализации; инкремент версии выполняется в репозитории через `version.Next()`
- `TagRepository.Save` — сигнатура изменена с `error` на `(Tag, error)`; репозиторий возвращает сохранённый агрегат с обновлённой версией
- `TagID`, `TagName`, `TagQuality`, `TagType` — переведены с type alias / primitive (`type Foo int`, `type Foo string`) на `struct { field T }`; исключает нечаянные приведения типов
- `TagQuality.IsValid()`, `TagType.IsValid()` — удалены; валидность гарантируется конструктором (`NewTagQuality`, `NewTagType` возвращают ошибку для неизвестных строк)
- `restapi/tags.go`, `restapi/widget_types.go` — 500-ошибки переведены на `sendInternalError(w, r, err)`: внутренние детали (SQL, стек) логируются, но не попадают в тело HTTP-ответа
- Тесты `tag_quality_test.go`, `tag_type_test.go` — граничный кейс `default` в `String()` восстановлен через `TagQuality{quality: 99}` / `TagType{tagType: 99}` вместо ранее использовавшихся raw-приведений типов

## [0.0.10] - 2026-05-12

### Added

- `internal/application/app_dto` — новый пакет с DTO уровня прикладного сервиса: `Tag`, `CreateTagInput`, `UpdateTagInput`; поля не имеют JSON-тегов, сериализация делегирована интерфейсному слою; фабричные функции `NewTag` и `NewTagList` конвертируют доменные агрегаты в DTO
- `internal/interface/restapi/rest_dto` — новый пакет с HTTP-DTO: `TagResponse`, `GetTagsResponse`, `CreateTagRequest`, `CreateTagResponse`, `UpdateTagRequest`, `UpdateTagResponse`; конвертеры `NewTagResponse`, `NewGetTagsResponse`, `NewCreateTagResponse`, `NewUpdateTagResponse`, `NewCreateTagInput`, `NewUpdateTagInput` изолируют маппинг между слоями

### Removed

- `internal/application/dto` — удалён единый DTO-пакет, смешивавший JSON-сериализацию и application-слой; его ответственность разделена между `app_dto` и `rest_dto`

### Changed

- `TagService` — все методы переведены на `app_dto`: `FindAllTags` возвращает `[]app_dto.Tag` вместо `dto.FindAllTagsResponse`; `FindTagByID` возвращает `app_dto.Tag` вместо `dto.Tag`; `CreateTag` принимает `app_dto.CreateTagInput` вместо `dto.CreateTagRequest` и возвращает полный `app_dto.Tag` вместо `dto.CreateTagResponse{ID}`; `DeleteTagByID` возвращает плоский `app_dto.Tag` вместо `dto.DeleteTagResponse{Tag: dto.Tag}`; `SetTagValueByID` принимает `app_dto.UpdateTagInput` вместо `dto.UpdateTagRequest` и возвращает `app_dto.Tag` вместо `dto.UpdateTagResponse{Version}`
- `restapi/tags.go` — хендлеры переведены на `rest_dto`: каждый хендлер конвертирует результат сервиса через соответствующую фабрику `rest_dto.New*`; удалены промежуточные переменные для ответов с ошибками; `UpdateTagRequest` больше не содержит поле `ID` — идентификатор передаётся только через path-параметр и подставляется в `NewUpdateTagInput`
- Тесты `tag_application_service_test.go` и `tags_test.go` — обновлены импорты и типы: `dto.CreateTagRequest` → `app_dto.CreateTagInput`, `dto.UpdateTagRequest` → `app_dto.UpdateTagInput`; тип возврата `createTagViaService` изменён с `dto.CreateTagResponse` на `app_dto.Tag`; декодирование HTTP-ответов использует `rest_dto.*` вместо `dto.*`
- Все doc-комментарии в `app_dto` и `rest_dto` переведены на английский язык

## [0.0.9] - 2026-05-08

### Added

- `EventBus` — реализация интерфейса `event.EventBus` в `internal/domain/event/event_bus.go`: хранилище подписчиков `map[EventType][]EventHandler`, потокобезопасность через `sync.RWMutex`; при `Publish` список обработчиков копируется под `RLock` и вызывается без блокировки, что исключает дедлок при вызове `Subscribe` из обработчика
- `MQTTEventBus` — декоратор над `event.EventBus` в `internal/infrastructure/mqtt/event_bus_mqtt.go`; при `Publish` вызывает внутреннюю шину и публикует событие в MQTT-брокер с топиком `events/<event_type>`; `MQTTClient` — порт-интерфейс с методом `Publish(topic string, payload []byte) error`, позволяющий подключить любую MQTT-библиотеку без изменения кода декоратора
- `NewTagService` — добавлен параметр `anEventBus event.EventBus`; `tagServiceImpl` хранит ссылку на шину и публикует события после успешных мутирующих операций: `TagCreatedEvent` в `CreateTag`, `TagDeletedEvent` в `DeleteTagByID`, `TagUpdatedEvent` в `SetTagValueByID`
- Юнит-тесты пакета `event` (`internal/domain/event/event_test.go`) — 33 теста:
  - `EventTimestamp`: создание без опций (метка близка к `now`), с фиксированным временем (`EventTimestampWithTime`), `String`, `ParseEventTimestamp` (валидная строка RFC3339 / невалидная → ошибка), `MustParseEventTimestamp` (валидная / паника)
  - `EventType`: `NewEventType` для всех 4 строк, для неизвестной строки → ошибка, round-trip `String` → `NewEventType` для каждого значения
  - `EventBus`: вызов подписчика при совпадении типа, изоляция по типу (другие подписчики не вызываются), несколько подписчиков для одного типа вызываются все, `Publish` без подписчиков не паникует
  - `SystemReadyEvent`, `TagCreatedEvent`, `TagUpdatedEvent`, `TagDeletedEvent`: тип, `TagID`, метка времени (не нулевая), `String` (не пустая), опция `WithTimestamp`
- Интеграционные тесты прикладного сервиса — 6 новых тестов на публикацию событий через `bus.Subscribe`:
  - `TestCreateTag_Success_PublishesTagCreatedEvent` — проверка типа события и `TagID`
  - `TestCreateTag_InvalidRequest_NoEventPublished` — при ошибке до `Save` событие не публикуется
  - `TestDeleteTagByID_Success_PublishesTagDeletedEvent` — проверка типа события и `TagID`
  - `TestDeleteTagByID_NotFound_NoEventPublished`
  - `TestSetTagValueByID_Success_PublishesTagUpdatedEvent` — проверка типа события и `TagID`
  - `TestSetTagValueByID_InvalidRequest_NoEventPublished`

### Fixed

- `SetTagValueByID` публиковал `TagDeletedEvent` вместо `TagUpdatedEvent` — исправлено

### Changed

- `NewTagService` — сигнатура расширена вторым параметром `anEventBus event.EventBus`; тесты в `tag_application_service_test.go` и `restapi/tags_test.go` обновлены: теперь передают `event.NewEventBus()`; вспомогательная функция `newServiceWithBus()` заменила `newServiceWithSpy()` — шина берётся из пакета `event`, подписка через `bus.Subscribe`
- `TagType` — добавлено нулевое значение `TagTypeUnknown TagType = iota` первым в блоке констант; `String()` получил явный `case TagTypeUnknown`; `NewTagType` возвращает `TagTypeUnknown` вместо `-1` при неизвестной строке; `IsValid()` исключает `TagTypeUnknown`
- `TagQuality` — добавлено нулевое значение `TagQualityUnknown TagQuality = iota` первым в блоке констант; аналогичные правки в `String()`, `NewTagQuality` и `IsValid()`
- `EventType` — добавлено нулевое значение `EventTypeUnknown EventType = iota` первым в блоке констант; `String()` получил явный `case EventTypeUnknown`; `NewEventType` возвращает `EventTypeUnknown` вместо `0` при неизвестной строке
- `Mode` — добавлена константа `ModeUnknown Mode = ""` первой в блоке констант (документирует нулевое значение типа `string`)
- Тесты `tag_type_test.go` — `TagType(-1)` в String-кейсе заменён на `TagTypeUnknown`; out-of-range значение `TagType(99)` покрывает `default`; `TagTypeUnknown` добавлен в список невалидных в `IsValid`-тесте
- Тесты `tag_quality_test.go` — аналогичные правки: `TagQuality(-1)` → `TagQualityUnknown`, добавлен `TagQuality(99)` для `default`, `TagQualityUnknown` в список невалидных
- Тесты `event_test.go` — `TestNewEventType_UnknownString_ReturnsError` расширен проверкой возвращаемого `EventTypeUnknown`; добавлен тест `TestEventType_String_Unknown`

## [0.0.8] - 2026-05-03

### Changed

- `TagKind` переименован в `TagType` во всём проекте: тип, константы (`TagTypeString`, `TagTypeBoolean`, `TagTypeInteger`), конструктор `NewTagType`
- `tag_kind.go` → `tag_type.go`, `tag_kind_test.go` → `tag_type_test.go`
- Метод `Tag.Kind() TagKind` переименован в `Tag.Type() TagType` в доменном интерфейсе и реализации
- Колонка `kind` переименована в `type` в схеме БД: обновлены DDL, индексы (`idx_tags_type`, `idx_tags_name_type`) и constraint (`chk_tags_type`)
- Все SQL-запросы в репозитории обновлены: `kind` → `type` в INSERT, UPDATE, SELECT, RETURNING
- DTO: поле `Kind string json:"kind"` переименовано в `Type string json:"type"` в `Tag` и `CreateTagRequest`
- Тесты: имена функций (`InvalidKind` → `InvalidType`, `ByKind` → `ByType`, `ForKind` → `ForType`, `UnknownKind` → `UnknownType`), переменные (`kind` → `tagType`, `newKind` → `newType`) и строки в логах (`"Kind:"` → `"Type:"`) обновлены во всех пакетах

## [0.0.7] - 2026-05-03

### Changed

- `cmd/combined_server/main.go`: все вызовы `fmt.Printf`/`fmt.Println` заменены на `slog.Error`/`slog.Info` со структурированными key-value аргументами
- `slogAdapter` — добавлен адаптер, реализующий интерфейс `gracedown.Logger` (`Println(v ...any)`); внутренние сообщения `gracedown` теперь выводятся через `slog.Info`
- `gracedown.NewManager` — передаётся `WithLogger(slogAdapter{})`, `gracedown` больше не использует `fmt.Println` напрямую

## [0.0.6] - 2026-05-03

### Added

- `DeleteTagByID` — метод удаления тега по ID в `TagService`; возвращает удалённый тег в `DeleteTagResponse`
- `SetTagValueByID` — метод обновления значения и качества тега в `TagService`; возвращает новую версию в `UpdateTagResponse`
- `DeleteByID` — метод удаления тега в `TagRepository`; возвращает `(Tag, error)`; реализация использует `DELETE ... RETURNING id, name, kind, value, quality, version`, что исключает лишний `SELECT`
- REST-хэндлер `DeleteTagByID` (`DELETE /api/v1/tags/{id}`) — удаление тега; 200 с телом удалённого тега, 404 при отсутствии, 400 при невалидном UUID
- REST-хэндлер `PatchTagValue` (`PATCH /api/v1/tags/{id}/value`) — обновление значения и качества; 200 с новой версией, 404 при отсутствии, 400 при невалидном UUID или теле
- Маршруты `DELETE /api/v1/tags/{id}` и `PATCH /api/v1/tags/{id}/value` зарегистрированы в роутере
- Bruno-коллекция: запросы `DELETE /api/v1/tags/{{TAG_ID}}` и `PATCH /api/v1/tags/{{TAG_ID}}/value`
- Интеграционные тесты репозитория — 4 теста для `DeleteByID`:
  - `TestDeleteByID_ExistingTag_ReturnsDeletedTag` — проверка всех полей возвращённого тега
  - `TestDeleteByID_ExistingTag_TagIsRemovedFromDB` — тег отсутствует в БД после удаления
  - `TestDeleteByID_ExistingTag_OtherTagsAreUnaffected` — прочие теги не затрагиваются
  - `TestDeleteByID_NotFound_ReturnsErrTagNotFound` — возвращает `ErrTagNotFound`
- Интеграционные тесты прикладного сервиса — 8 тестов:
  - `TestDeleteTag_ExistingTag_ReturnsDeletedTag`, `TestDeleteTag_ExistingTag_TagIsRemovedFromDB`, `TestDeleteTag_NotFound_ReturnsWrappedErrTagNotFound`
  - `TestSetTagValueByID_ValidUpdate_ReturnsIncrementedVersion`, `TestSetTagValueByID_ValidUpdate_ValueAndQualityAreUpdated`, `TestSetTagValueByID_NotFound_ReturnsWrappedErrTagNotFound`, `TestSetTagValueByID_InvalidQuality_ReturnsError`, `TestSetTagValueByID_InvalidValueForKind_ReturnsError`
- Интеграционные тесты HTTP-хэндлеров — 9 тестов:
  - `TestDeleteTagByID_ExistingTag_Returns200WithDeletedTag`, `TestDeleteTagByID_ExistingTag_TagIsRemovedFromDB`, `TestDeleteTagByID_NotFound_Returns404WithProblemDetails`, `TestDeleteTagByID_InvalidUUID_Returns400WithProblemDetails`
  - `TestPatchTagValue_ValidUpdate_Returns200WithVersion`, `TestPatchTagValue_ValidUpdate_ValueAndQualityAreUpdated`, `TestPatchTagValue_NotFound_Returns404WithProblemDetails`, `TestPatchTagValue_InvalidUUID_Returns400WithProblemDetails`, `TestPatchTagValue_InvalidJSON_Returns400WithProblemDetails`

### Changed

- `Tag.UpdateValue` переименован в `Tag.SetValue` в доменном интерфейсе и реализации; тест-функции переименованы соответственно (`TestTag_SetValue_*`)
- `TagService.DeleteTag` переименован в `TagService.DeleteTagByID`
- `TagRepository.Delete` переименован в `TagRepository.DeleteByID`; сигнатура изменена с `error` на `(Tag, error)` — сервис делает один вызов репозитория вместо двух (`FindByID` + `Delete`)
- `Save` в PostgreSQL-репозитории: добавлена явная обработка ошибки `RowsAffected()` для INSERT и UPDATE (для postgres всегда `nil`)
- `.github/workflows/go.yaml`: путь сборки исправлен с `./cmd/combined/` на `./cmd/combined_server/` — директория была переименована

## [0.0.5] - 2026-05-01

### Added

- Интеграционные тесты прикладного сервиса `tag` (`internal/application/tag_application_service_test.go`) — 10 тестов:
  - `TestCreateTag_ValidIntegerTag_ReturnsID` — создание тега типа `integer`, проверка ненулевого UUID в ответе
  - `TestCreateTag_ValidStringTag_ReturnsID` — создание тега типа `string`
  - `TestCreateTag_ValidBooleanTag_ReturnsID` — создание тега типа `boolean`
  - `TestCreateTag_InvalidKind_ReturnsError` — невалидный kind возвращает ошибку
  - `TestCreateTag_InvalidQuality_ReturnsError` — невалидное quality возвращает ошибку
  - `TestCreateTag_InvalidValueForKind_ReturnsError` — значение несовместимо с типом (строка вместо числа) возвращает ошибку
  - `TestFindAllTags_EmptyDB_ReturnsEmptyList` — пустая БД возвращает пустой список
  - `TestFindAllTags_MultipleTags_ReturnsAll` — возвращает все сохранённые теги
  - `TestFindTagByID_ExistingTag_ReturnsTag` — возвращает тег с корректными полями в DTO
  - `TestFindTagByID_NotFound_ReturnsWrappedErrTagNotFound` — возвращает ошибку с обёрнутым `ErrTagNotFound`
- Интеграционные тесты HTTP-хэндлеров `tag` (`internal/interface/restapi/tags_test.go`) — 8 тестов; запросы проходят через `router.ServeMux().ServeHTTP()`, что корректно заполняет path-параметры:
  - `TestGetTags_EmptyDB_Returns200WithEmptyList` — пустая БД, статус 200, пустой массив тегов
  - `TestGetTags_WithTags_Returns200WithAll` — теги в БД, статус 200, все теги в ответе
  - `TestGetTagsByID_ExistingTag_Returns200WithTag` — существующий тег, статус 200, корректные поля
  - `TestGetTagsByID_NotFound_Returns404WithProblemDetails` — несуществующий UUID, статус 404, тело в формате RFC 9457
  - `TestGetTagsByID_InvalidUUID_Returns400WithProblemDetails` — невалидная строка в `{id}`, статус 400, тело RFC 9457
  - `TestPostTags_ValidBody_Returns201WithID` — корректный JSON, статус 201, ненулевой UUID в ответе
  - `TestPostTags_InvalidJSON_Returns400WithProblemDetails` — невалидный JSON, статус 400, тело RFC 9457
  - `TestPostTags_InvalidKind_Returns500WithProblemDetails` — невалидный kind, статус 500, тело RFC 9457
- Юнит-тесты `ProblemDetails` (`internal/interface/restapi/problems_test.go`) — 11 тестов:
  - `TestProblemDetails_MarshalJSON_*` — 4 теста: стандартные поля попадают в JSON, пустой `detail` опускается, extensions сериализуются, extension не перебивает стандартное поле
  - `TestProblemDetails_UnmarshalJSON_*` — 2 теста: стандартные поля парсятся, неизвестные поля попадают в `Extensions`
  - Фабрики: `TestNewBadRequest_*`, `TestNewNotFound_*`, `TestNewConflict_*`, `TestNewValidationError_*`, `TestNewInternalError_*` — проверка статуса, типа, полей и extensions

## [0.0.4] - 2026-04-30

### Added

- Интеграционные тесты для инфраструктурного репозитория `tag` (`internal/infrastructure/postgres/repositories/tag_repository_postgres_test.go`) — 8 тестов:
  - `TestNextID_ReturnsUniqueIDs` — генерирует уникальные идентификаторы
  - `TestSave_NewTag_InsertsSuccessfully` — INSERT новой записи без ошибок
  - `TestSave_DuplicateID_ReturnsError` — повторный INSERT с тем же ID возвращает ошибку
  - `TestSave_ExistingTag_UpdatesSuccessfully` — UPDATE существующей записи, проверка новых значений
  - `TestSave_StaleVersion_ReturnsError` — оптимистичная блокировка: UPDATE с несовпадающей версией возвращает ошибку
  - `TestFindByID_ExistingTag_ReturnsTag` — возвращает тег с корректными полями
  - `TestFindByID_NotFound_ReturnsErrTagNotFound` — возвращает `ErrTagNotFound` при отсутствии записи
  - `TestFindAll_EmptyDB_ReturnsEmptySlice` — возвращает пустой срез при пустой таблице
  - `TestFindAll_MultipleTags_ReturnsAll` — возвращает все сохранённые теги
- Зависимость `github.com/pressly/goose/v3 v3.27.1` — применение SQL-миграций в тестовом окружении
- Тесты используют `testcontainers-go/modules/postgres`: контейнер поднимается в `TestMain`, миграции из `internal/infrastructure/postgres/migrations/` применяются через goose

## [0.0.3] - 2026-04-30

### Added

- Юнит-тесты для доменного агрегата `tag` (`internal/domain/tag/`) — 9 файлов, 63 теста:
  - `tag_id_test.go` — генерация UUID, парсинг, паника для невалидных строк, roundtrip
  - `tag_name_test.go` — создание имени, `String()`
  - `tag_kind_test.go` — парсинг строк, `String()`, `IsValid()`
  - `tag_quality_test.go` — парсинг строк, `String()`, `IsValid()`
  - `tag_value_boolean_test.go` — конструктор из `string`/`bool`/`int`, ошибки, `Value()`, `String()`
  - `tag_value_integer_test.go` — конструктор из `string`/`bool`/`int`, ошибки парсинга, `Value()`, `String()`
  - `tag_value_string_test.go` — конструктор из `string`/`bool`/`int`, конвертации, `Value()`, `String()`
  - `tag_value_test.go` — фабрика `NewTagValue`: диспетчеризация по `TagKind`, неизвестный kind, несовместимый тип
  - `tag_test.go` — создание агрегата, все геттеры, `IncrementVersion`, `UpdateValue`
- `.github/workflows/go.yaml` — GitHub Actions CI: запуск тестов и сборка при push/PR в ветку `path`

### Changed

- `.gitverse/workflows/go.yaml` — исправлены три проблемы: триггер `workflow_dispatch` заменён на `push`/`pull_request` для ветки `path`; версия Go зафиксирована через `go-version-file: go.mod` вместо явного `1.23`; путь сборки `cmd/server/main.go` (несуществующий) исправлен на `./cmd/combined/`

## [0.0.2] - 2026-04-28

### Added

- `FindTagByID` — реализован метод получения тега по UUID в сервисном слое (`TagService`)
- `FindByID` — реализован метод в репозитории PostgreSQL: SQL-запрос по `id`, маппинг строки в доменные value objects, возврат `ErrTagNotFound` при отсутствии записи
- `ErrTagNotFound` — добавлена сентинель-ошибка в доменный пакет `tag`
- Bruno-коллекция: добавлен запрос `GET /api/v1/tags/{{TAG_ID}}`
- `emergencyExit` — вспомогательная функция для аварийного завершения с корректным shutdown всех компонентов

### Changed

- `TagService.FindTagByID` — тип параметра изменён с `int` на `tag.TagID`
- `gracedown` обновлён до версии `v0.0.0-20260423215449-38425123f880`; переход с `WaitForSignal` на `WaitForSignalAndShutdown`, коды выхода вынесены в пакет `gracedown`
- Порядок инициализации в `main.go`: инфраструктурные компоненты (БД) поднимаются раньше интерфейсных (HTTP-серверы); регистрация shutdown-хука для БД перенесена до `Ping()`, чтобы соединение закрывалось при аварийном завершении
- `sendJSONResponse` — сериализация тела теперь выполняется до отправки заголовков, что позволяет вернуть `500` при ошибке маршалинга
- `GetTags`, `GetTagsByID` — упрощены: ручной `json.Marshal` + `w.Write` заменены на `sendJSONResponse`
- `PostTags` — исправлен отсутствующий `return` после ошибки `CreateTag`; без него параллельно уходили ответы `500` и `201`
- `GetTagsByID` зарегистрирован в роутере: `GET /api/v1/tags/{id}`

## [0.0.1] - 2026-04-22

- Basic tag functionality
