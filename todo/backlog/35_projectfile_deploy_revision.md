# ProjectFile, Deploy, Revision, Discard
Status: ready-for-human
Blocked by: 32, 33, 34

Слой B. Термины — `CONTEXT.md` (ProjectFile, Deploy, Revision, Discard).

- Формат ProjectFile (контракт `contract/project/v0`, ADR 0005): весь Draft проекта — Scenes с Widgets, определения Tag, Devices — плюс содержимое используемых WidgetType из закреплённых Library Release (задача 33) и `schema_version`.
- Deploy: Engineering node сериализует Draft в ProjectFile и передаёт Runtime node; тот атомарно создаёт Revision (`runtime.revisions(project_id, n, deployed_at, config jsonb)`), присваивает номер и переключает Operation.
- WidgetType больше не действует мгновенно — только через Release (задача 33) и Deploy.
- Перед Deploy Engineer видит, какие runtime-сущности пострадают (исчезли Scene/WidgetType); такие Widgets удаляются. Конфликт имён Tag с runtime-тегом → Deploy отклоняется.
- Discard: Draft сбрасывается к текущей Revision, полученной через API Runtime node.
- Engineering node хранит копию последнего отправленного ProjectFile.
- Запись Deploy в Journal — в задаче 42.
- REST, Bruno, тесты, `CHANGELOG.md`.

## Comments

- 2026-10-04: бывший тикет 03 задачи 36. Решения: Q1 (Draft + Deploy, а не живое редактирование), Q8 (Deploy публикует весь Draft атомарно → Revision), Q10 (Draft автосохраняется на сервере, кнопки «Сохранить» нет; явные действия — Deploy и Discard), Q15 (runtime-Widget переживает Deploy; удаляется, только если исчезли его Scene или WidgetType, с предупреждением перед Deploy), Q16 (Deploy с конфликтом имён отклоняется), Q20 (Revision — неизменяемый jsonb-документ). Зависимость от Journal снята: запись Deploy в Journal добавит задача 42.
