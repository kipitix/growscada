# Разнесение узлов по сети
Status: ready-for-human
Blocked by: 35, 50

- Запуск бинарника с ролями Engineering, Runtime или обеими (конфигурация).
- HTTP-реализация интерфейса Engineering→Runtime: Deploy, текущая Revision, runtime-сущности.
- Отдельные БД по строкам подключения.
- Аутентификация между узлами — после задач 49/50.
- Сервер с одной ролью: в его БД серверная схема `public` и схема этой роли (ADR 0011).
- Очистка удалённого Project без FK между схемами. Сейчас поздний upsert определения Tag в Runtime (после `RemoveProjectTagStates`; источники: upsert после коммита в `CreateTag` и job `sync_runtime_tag_definitions`) удерживает строку `public.projects` через FK `fk_runtime_tags_project_id`: `ProjectRepository.Delete` распознаёт его как `ErrProjectNotEmpty`, и `remove_deleted_project` повторяется. С отдельными БД FK нет: строка Project удалится, состояния останутся навсегда. Гарантию даёт сам Runtime: `RemoveProjectTagStates` в одной транзакции удаляет состояния и записывает надгробие Project (`runtime.removed_projects`), `UpsertTagDefinitions` не вставляет определения Project с надгробием (решить: ошибка или пропуск) — проверка и вставка одним запросом. Затем убрать FK `fk_runtime_tags_project_id`, его распознавание в `project_repository_postgres.go` и комментарии про «поздний upsert удерживает строку»; обещание — в доке порта `RuntimeNode` (замечание ревью задачи 32, п. 10).

## Comments

- 2026-10-04: бывший тикет 13 задачи 36 (там был отложен).
- 2026-10-27: добавлен пункт об очистке Project без FK между схемами (ревью задачи 32).
