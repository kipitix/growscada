# 410 Gone на старых путях контента вместо голого 404
Status: needs-triage

Появилось при ревью задачи 31 (находка №1). В REST API 0.4 Tags и Scenes (вместе с Widgets) переехали под `/api/v0/projects/{project_id}/…`. На старые пути `/api/v0/tags…` и `/api/v0/scenes…` `http.ServeMux` отвечает своим текстовым 404: не Problem Details и без подсказки, куда переехал ресурс. По правилам проекта версию подняли верно (MINOR, пока MAJOR = 0), но старые клиенты ломаются молча:

- Старый `devicelink`/симулятор (0.3): `PATCH /api/v0/tags/{id}/value` → 404 → `apiclient.ErrNotFound` → `devicelink.ErrTagNotFound`, и симулятор навсегда перестаёт писать каждый тег («tag deleted on the server, no longer simulated»). Предупреждения о версии нет: для клиента 0.3 версия 0.4 совместима.
- Старый growctl получает «not found» без объяснения.
- Старые подписчики `GET /api/v0/events` без `?project=` молча перестают получать `tag_*`, `scene_*`, `widget_*`. Статусом ответа это не исправить: поток открывается.

Встроенный UI не страдает: он собран из того же бинарника.

Требования:

- Маршруты `/api/v0/tags`, `/api/v0/tags/`, `/api/v0/scenes`, `/api/v0/scenes/` (любой метод) отвечают 410 Gone в формате Problem Details (`NewGone`). Деталь говорит, что ресурс переехал под `/api/v0/projects/{project_id}/…`. Образец — `preVersioningGone` для `/api/v1` в `restapi/router.go`.
- У старого `devicelink` 410 — не `ErrNotFound` и не временная ошибка: запись завершается ошибкой и повторяется на следующем тике, а не теряет тег. Проверить это тестом.
- Шим удаляется вместе с `/api/v1` при выходе API 1.0. Отметить это в комментарии, как у `preVersioningPathPrefix`.
- Решить, нужен ли общий 404 в формате Problem Details для любого неизвестного пути под `/api/v0/`. К переезду он не относится, но вписывается в ту же правку.
- Bruno: каждый новый маршрут покрыть запросом с проверкой 410, заголовка `GrowSCADA-Schema-Version` и `application/problem+json` (по образцу `get_pre_versioning_path.yml`); статическая проверка `TestBrunoCollection` требует покрытия каждого маршрута.
- Контракт и форма ответов не меняются: SchemaVersion не поднимать.
