# Undo/redo в PWA

Status: ready-for-agent
Blocked by: 38_3
Parent: 38_draftchange_undo_redo
Part: 4/4
Closes parent: yes

Часть 4 задачи 38. Источник истины — `todo/backlog/38_draftchange_undo_redo.md`, решение — ADR 0017. Маршруты `draft-changes` готовы в 38_3; здесь — UI Project и Library.

**За рамками:** серверная часть (38_1–38_3).

## UI

- В шапке Project и Library рядом со строкой EditLock — кнопки `Undo` и `Redo`. Подсказка (tooltip/label) — `undo_description`/`redo_description` из `GET …/draft-changes`; пустое описание — кнопка выключена. Без замка обе выключены с подсказкой, как остальные правки.
- Горячие клавиши `Ctrl+Z` и `Ctrl+Shift+Z` (и `Cmd` на macOS) в режимах Project и Library; не перехватывать, когда фокус в поле ввода. Действие — тот же POST, что и кнопка.
- Состояние стека читается `GET …/draft-changes` при загрузке режима, на `EventResync`, после каждой успешной правки и после undo/redo; ответ даёт `version`, `can_undo`, `can_redo` и описания. Потеря замка (toast и read-only) выключает кнопки, стек при этом не теряется.
- Undo/redo шлют `{version}` из последнего ответа; 409 устаревшей версии — перечитать состояние и содержимое (как 409 правки); 409 пустого стека — toast и обновление состояния.
- Содержимое после undo/redo обновляется по событиям, как после обычной правки: `tag_*`, `scene_*`, `widget_*`, `library_pin_*`, `device_*`, `environment_*`; для Library — `widget_type_*`. Отдельного `draft_changed` нет.
- Одна Version Draft на Draft: заменить `sceneVersions` (Project) и карту версий WidgetType (Library) на одну `draftVersion`, которую обновляет каждый ответ и сбрасывает `uiutil.Versions` на 409/`EventResync`; `lock_version` EditLock остаётся отдельным. Ответы другого Project/Library по-прежнему отбрасываются (`uiutil.LoadFor`, `SetProject`).
- `apiclient`: методы из 38_3; `contract/api/v0` — без изменений сверх 38_3.

## Тесты и проверка

- UI-тесты (`ui/uitest`, `servertest`): кнопки и подсказки при пустом/непустом стеке, без замка; undo/redo кнопкой и клавишей; обновление содержимого по событиям; 409 устаревшей версии и пустого стека; переключение Project/Library сбрасывает состояние; одна версия Draft в записях.
- Браузер (`chrome-devtools`, правило 9): Project — правка, undo, redo, перезагрузка страницы (стек пережил), Check in/out и перехват (стек пережил), Deploy (стек очищен); Library — то же с Release и Discard; консоль без ошибок.
- `AGENTS.md`: кнопки, клавиши, состояние стека, одна версия; `CHANGELOG.md`.
