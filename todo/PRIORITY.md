# Очередь приоритетов

Индекс для людей и агентов; сами задачи — в `todo/backlog/`. Правило приоритета описано в `docs/agents/issue-tracker.md`: `Priority: 1` важнее номера файла, отсутствие строки = `3`.

`/kx_auto` читает порядок из таблицы `## P1` ниже: имя файла — в обратных кавычках, строки — по порядку; после P1 идёт остальной backlog по номеру. Задача, уже лежащая в `todo/done/`, пропускается.

Обновлено: 2026-11-09.

## P1 — взять в работу первыми

| Порядок | Задача | Что | Зависит от |
|---|---|---|---|
| 1 | `73_ui_library_project_pickers.md` | Страницы выбора Library/Project вместо выпадающего списка; описание у обеих сущностей; unsaved-бейдж в списке | — |
| 2 | `72_1_remove_editlock.md` | Убрать Check in/out и EditLock; конкурентность — Version Draft | — |
| 3 | `72_save_release_deploy.md` | Спека модели; Commit → Release → Deployment | — |
| 3′ | `72_9_rename_revision_to_deployment.md` | Переименовать Runtime Revision → Deployment | — |
| 4 | `72_2_commit_save.md` | Save → Commit; история, заголовок, автор | 72_1 |
| 5 | `72_3_project_releases.md` | Project Release из Commit; список | 72_2 |
| 6 | `71_release_pinned_by_history.md` | Гибрид: живой пин/работающая Deployment блокируют удаление Library; старая история — деградация | 72_2, 72_3, 72_9 |
| 7 | `72_4_deploy_release.md` | Deploy Project Release, running-маркер, Rollback/Discard | 72_3, 72_9 |
| 7′ | `72_5_library_commit_release.md` | Library Release из Commit; пины; защита Library | 72_2, 71 |
| 8 | `72_6_project_ui_save_releases.md` | Project UI: Save, история, Releases, Deploy | 72_4 |
| 8′ | `72_7_library_ui_save_releases.md` | Library UI: Save, Commit, Release, пины | 72_5, 72_1 |
| 9 | `72_8_migration_tools_docs.md` | Миграции, growctl, Bruno, ADR/GLOSSARY/AGENTS/CHANGELOG | 72_1–72_7, 72_9 |

`72_9` можно делать параллельно с `72_1`/`72_2`, но до `72_4`. `72_4` и `72_5` можно вести параллельно после `72_3`/`71`; `72_6` и `72_7` — после своих серверных частей. `72_8` закрывает цепочку.

## P3 — остальное

Задачи 44–70 остаются в прежнем порядке (по номеру) и не блокируют P1. Исключения — `71` выше: она стала P1, потому что каждый Commit Project'а теперь называет закреплённые Library Release, и `73` — следующий шаг доработки UI после 72_6/72_7.

## Почему так

- Задача 73 — страницы выбора Library/Project — идёт первой: это следующий UI-шаг после 72_6/72_7, и от старых P1 она не зависит.
- Сначала убираем неочевидный замок (72_1) — это самостоятельный видимый результат.
- Параллельно переименовываем Runtime Revision в Deployment (72_9), чтобы модель была Commit → Release → Deployment, без четвёртого неоднозначного термина.
- Затем Save/Commit (72_2), потому что Release и Deploy строятся на нём.
- Release (72_3/72_5) и Deploy (72_4) разделены: Project Release деплоится, Library Release только закрепляется.
- Защита Library (71) идёт сразу после Project Release: она не даёт истории сохранений оборвать Release.
- UI идёт после серверных частей, чтобы не переделывать экраны дважды.
- Миграции/инструменты/документы — в конце, когда форма API и модели стабилизировалась.
