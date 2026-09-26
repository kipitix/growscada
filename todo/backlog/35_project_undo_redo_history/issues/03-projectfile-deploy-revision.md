# ProjectFile, Deploy, Revision, Discard

Type: task
Blocked by: 01, 02

- Формат ProjectFile: весь Draft (WidgetTypes, Scenes с Widgets, определения Tag) + `schema_version` (связь с задачей 16).
- Deploy: Engineering node сериализует Draft в ProjectFile и передаёт Runtime node; тот атомарно создаёт Revision (`runtime.revisions(n, deployed_at, config jsonb)`), присваивает номер, пишет запись в Journal и переключает Operation.
- Library (WidgetType) больше не действует мгновенно — только через Deploy.
- Перед Deploy Engineer видит, какие runtime-сущности пострадают (исчезли Scene/WidgetType); такие Widgets удаляются с записью в Journal. Конфликт имён Tag с runtime-тегом → Deploy отклоняется.
- Discard: Draft сбрасывается к текущей Revision, полученной через API Runtime node.
- Engineering node хранит копию последнего отправленного ProjectFile.
- REST, Bruno, тесты, CHANGELOG.md.
