# Engineering-нода: сборка ProjectFile, Deploy, preview, Discard, снятие моста
Status: ready-for-agent
Blocked by: 35_3

Часть 4 задачи 35. Engineering-нода собирает ProjectFile из Draft, деплоит его через порт, показывает предупреждение перед Deploy и умеет сбрасывать Draft к текущей Revision. Здесь же умирает временный мост Draft→Runtime.

Источник истины — `todo/backlog/35_projectfile_deploy_revision.md` (гриллинг 2026-11-03, Q4, Q6, Q7, Q9, Q10): это **спецификация**, а не отдельная работа.

## Что сделать

### Сборка ProjectFile из Draft

- Один сборщик (`application`, Engineering) над `ProjectScope` и пинами: Tags с поставщиком, Devices, Environments и пары, Scenes с Widgets, **только используемые** WidgetType из закреплённых Release (`ProjectScope.LibraryPins().FindPinnedWidgetType`), список пинов `{library_id, release}`.
- Канонизация: списки сортируются по ID, `config`/`params` — нормализованный JSON; одна и та же Draft-конфигурация даёт байт-в-байт одинаковый документ. Без этого индикатор из Q6 не работает.
- Висячие ссылки (Widget ссылается на тип не из закреплённого Release, Tag-поставщик не Device проекта) — ошибка сборки, а не молчаливый пропуск.

### DeployService (Engineering)

- `Deploy(projectID)`: собрать документ → `RuntimeNode.FindCurrentRevision` → если документ совпал с текущим — 409 `nothing_to_deploy` (Q9); иначе `RuntimeNode.Deploy` → сохранить `engineering.project_files` (часть 2) → ответ `{number, deployed_at}`.
- Сбой записи копии после успешного Deploy — 500 с явным текстом «Revision N активна, копия не сохранена»; повторный Deploy не отклонится как «без изменений», потому что сравнение идёт с Revision на Runtime (Q8). Копия — кэш для UI, корректность от неё не зависит.
- `DeployPreview(projectID)`: `{current_revision (number, deployed_at; может отсутствовать), unchanged, dropped_widgets []}`. `dropped_widgets` сейчас пуст всегда — runtime-сущности появятся в задаче 48; форма API и его тест фиксируются здесь (Q9).
- Индикатор неразвёрнутых изменений: `DeployState{deployed *appdto.Revision, differs bool}` — собрать документ и сравнить канонический JSON с копией.

### Discard

- `Project`: `StartDiscard()`, `FinishDiscard()`, `IsDiscarding()`; события `ProjectDiscardStartedEvent` и `ProjectUpdatedEvent` (как `Library.StartDiscard`/`FinishDiscard`, ADR 0012).
- Пока пометка стоит — любая мутация Draft (Tag, Scene, Widget, Device, Environment, пара, пин) отказывает 409: проверка в том же месте, где `aggregateStore.lockLiveProject` держит транзакцию создания.
- `DiscardService.Discard(projectID)` → 409, если Revision нет; иначе `StartDiscard` и сразу ответ.
- `NewDiscardProjectPolicy` (policy, ключ `project:<id>`): читает текущую Revision через `RuntimeNode`, восстанавливает Draft по одному агрегату в транзакции, порядок — **пины → Devices → Environments → пары → Tags → Scenes с Widgets** (сцена проходит `checkPinned`, поэтому пины первыми); лишнее удаляется, недостающее создаётся с теми же ID; пины — штатными методами агрегата, но **без `ImpactOf`** и без ожидаемой версии (Q10); обычный `Save` даёт Version+1 и штатные события; пометка снимается последней.
- Runtime (состояния Tag, токены) Discard не трогает.

### Снятие моста Draft→Runtime

- Убрать upsert определения в `CreateTag` (после коммита), удаление состояния в `DeleteTagByID`, policy `NewRuntimeTagDefinitionsPolicy` и её регистрацию в `main.go`; поправить доки `RuntimeNode` и `AGENTS.md`-абзац про мост.
- Следствие: запись значения в Tag, которого нет в работающей Revision, → 404. Тесты, которые писали значения, деплоят проект в setup через `DeployService`.

## Проверка

- `make build`, `make test`.
- Тесты: Deploy создаёт ревизию и копию у Engineering; повторный Deploy без изменений → 409; правка одного Widget → `differs` и Deploy создаёт ревизию N+1; сбой копии → 500, но Revision активна; Discard восстанавливает удалённый Widget/Scene/Tag/Device/пин с теми же ID, удаляет добавленное, не трогает состояния Tag; мутация Draft во время Discard → 409; Discard без Revision → 409.
- `go test ./internal/server/roles_test.go` — импорты ролей не нарушены.

## Вне объёма

- REST-маршруты, apiclient, UI, Bruno, ADR/CHANGELOG — часть 5.
