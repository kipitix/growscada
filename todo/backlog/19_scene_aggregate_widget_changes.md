# Изменения Widget через агрегат Scene
Status: needs-triage

Появилось в архитектурном ревью 2026-10-05. По `CONTEXT.md` Scene — единственная граница консистентности для себя и своих Widget. На деле изменения Widget идут мимо агрегата: это отдельные методы репозитория, и правила Scene разнесены по трём модулям.

## Что не так сейчас

- В `scene.Scene` нет поведения для Widget. `SceneRepository` (`domain/scene/scene_repository.go`) — 12 методов, среди них `AddWidget`, `UpdateWidget`, `DeleteWidget`, `FindWidgetsByTypeID`.
- Правила Scene живут в трёх местах:
  - `SceneService` (`application/scene_application_service.go`) проверяет порты по WidgetType и сверяет Version;
  - `scene_repository_postgres.go` повышает Version (`bumpSceneVersion`) и переводит нарушение FK в `ErrWidgetTypeNotFound`;
  - сервис ещё раз оборачивает эту ошибку в invalid input.
- `DeleteWidget` повышает Version без проверки: `UPDATE scenes SET version = version + 1 WHERE id = $1`. Add и Update при этом проверяют.
- `UpdateScene` сверяет Version дважды: в сервисе и в SQL (`WHERE id = $5 AND version = $6`).
- `FindByID` читает Scene и её Widget двумя запросами вне транзакции и может увидеть Scene в разные моменты времени.
- У `FindWidgetsBySceneID` и `FindWidgetByID` в репозитории нет вызывающих, кроме тестов.

## Что сделать

- Перенести правила Widget в агрегат Scene: добавление, изменение, удаление, проверка PortBinding по InputPort WidgetType, однократное повышение Version.
- Сократить интерфейс `SceneRepository` до чтения Scene целиком и сохранения Scene целиком: Widget сохраняются диффом в одной транзакции, Version сверяется один раз. Неиспользуемые методы удалить.
- Чтение Scene с Widget — в одной транзакции.
- Изменение Scene должно отдавать состояние «до» и «после». Это понадобится DraftChange (задача 38).
- Тесты:
  - правила Widget тестируются на агрегате, без Postgres;
  - репозиторий тестируется на сохранении и чтении Scene целиком (testcontainers);
  - тесты удалённых методов репозитория удаляются, а не дублируются.

## Приёмка

- REST API и контракт `contract/api/v0` не меняются, Bruno-коллекция проходит как раньше.
- Удаление Widget с устаревшей Version Scene возвращает 409.
- `make build` и `make test` проходят.
