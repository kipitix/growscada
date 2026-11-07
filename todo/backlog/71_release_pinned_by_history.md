# Пропавшая Release в истории: удаление Library разрешено, Discard деградирует

Status: ready-for-agent
Priority: 1
Blocked by: 72_2, 72_3, 72_9

Найдено ревью задачи 35_4 (подозрение, подтверждённое чтением кода; в ветке 35_4 не чинилось — решение затрагивает ADR 0007 и межнодовую проверку). После 72 (Save → Commit → Release → Deploy) проблема стала шире: теперь **каждый Save Project'а** фиксирует закреплённые Library Release в Commit'е, а Project Release копирует их.

`Discard` Проекта восстанавливает Draft по последнему Commit'у, `Revert to deployed` — по работающей Deployment; оба используют `restorePins`, который находит Release по номеру (`p.release`, `application/discard_project_policy.go`). При этом `NewLibraryCleanupPolicy` удаляет Library вместе с её Releases, как только её никто не пинит (ADR 0007, `AGENTS.md`, «Libraries»). Отпинить Library и удалить её можно, пока старый Commit/Project Release или работающая Deployment всё ещё называет один из её Releases.

Следствие: `restorePins` получает `library.ErrReleaseNotFound`, job `discard_project` ретраится вечно (PolicyRunner повторяет ошибку до бесконечности), пометка Discard не снимается (или Draft остаётся в неконсистентном состоянии), любая запись Draft отвечает 409, отменить Discard нельзя.

## Решение (2026-11-07): гибрид

- **Блокируют удаление Library:**
  - живой Project Pin (`engineering.library_pins`) — как сегодня (ADR 0012);
  - **работающая Deployment** любого Project, если её `ProjectFile` называет Release этой Library. Это нужно, чтобы Discard к текущей Deployment всегда мог восстановить пины.
    - Если текущая Deployment сделана из Project Release (`release_number` задан), достаточно `engineering.project_release_library_pins` этой Release; Runtime не нужен.
    - Если текущая Deployment сделана из файла (`release_number = nil`), нужен вопрос к Runtime node: порт `RuntimeNode` расширяется запросом `DeploymentNamesLibraryRelease(libraryID, releaseNumber)` (или Runtime хранит производную таблицу пинов Deployment). Ответ — только да/нет, без чтения Engineering-схемы.
- **Не блокируют удаление:** старые Commit и Project Release, которые уже не работают. Их содержимое самодостаточно (копии WidgetTypes в `ProjectFile`), поэтому History/Playback рендерятся и после удаления Library. Удаление Library с такой историей разрешено; UI предупреждает: «в истории останется ссылка на удалённую Library».
- **Деградация старого восстановления (Discard к Commit'у / Revert к Deployment):** если `restorePins` не находит Release (Library удалена), пин пропускается:
  - событие/лог `library_release_missing` (или существующий механизм предупреждений; имя согласовать с ADR 0009);
  - Draft остаётся без этого пина; `DeployPreview`/`DeployState` показывают «в истории не хватает Release»;
  - job `discard_project` **завершается**, а не ретраится вечно; Draft снова доступен для записи.
- **Library без истории и без живых пинов** удаляется как сегодня.
- Таблицы `engineering.commit_library_pins` и `engineering.project_release_library_pins` (72_2/72_3) остаются: по ним UI предупреждает об истории и определяется, какая Release работающей Deployment названа. Они больше не являются запретом на удаление старых ссылок.

## Что сделать

- `NewLibraryCleanupPolicy`: проверять живой `library_pins` и работающую Deployment (через `project_release_library_pins` или Runtime-порт); старые Commit/Release не проверять.
- `NewDiscardProjectPolicy.restorePins`: при `library.ErrReleaseNotFound` пропустить пин, записать предупреждение и завершить job успешно; не оставлять пометку Discard и не ретраить.
- `DeployPreview`/`DeployState` (или их преемник в 72_4): показать отсутствующие Release, если Draft восстанавливался из старой Deployment.
- Тесты:
  - удаление Library при живом Project Pin — 409;
  - удаление Library, названной работающей Deployment — 409;
  - удаление Library, названной только старым Commit/Project Release — 200, история рендерится;
  - Discard к старой Deployment после удаления Library: пин пропущен, есть предупреждение, job завершён, Draft доступен;
  - `PolicyRunner` не оставляет вечно ретраящийся job (ключ `project:<id>`, `discard_project`);
  - удаление Library без истории и пинов — 200.
- Bruno: удаление с живым пином/работающей Deployment — 409; удаление только со старой историей — 200; Discard после удаления — 200 с предупреждением.
- Документы: ADR 0007/0012/0021 (гибрид), `GLOSSARY.md` (Library Release, Library Pin, Deployment), `AGENTS.md` (Libraries: что блокирует удаление), `CHANGELOG.md`.

## Ссылки

- задача 35, часть 4 (`todo/done/35_4_engineering_deploy_discard.md`) — ревью, находка «Discard встаёт навсегда, если Release пина исчез»;
- задача 35, часть 2 (`todo/done/35_2_revision_storage.md`) — `runtime.revisions` (теперь `runtime.deployments`) хранит ProjectFile с пинами как провенанс;
- задача 72 и части 72_2/72_3/72_5 — Commit/Release и таблицы пинов.

## Comments

- 2026-11-07: после 72 каждый Commit Project'а содержит пины Library Release; проблема расширена с работающей Deployment на Commit/Project Release/Deployment. Приоритет 1, блокирует 72_5.
- 2026-11-07 (решение пользователя): **гибрид**. Живой пин и работающая Deployment блокируют удаление; старая история — нет. Discard к старой Deployment пропускает исчезнувшую Release с предупреждением и не ретраится вечно. Library с историей можно удалить.
