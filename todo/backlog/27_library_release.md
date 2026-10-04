# Library и Library Release
Status: ready-for-human
Blocked by: 26

Слой B. Термины — `CONTEXT.md` (Library, Library Release, Release, Draft); решение — ADR 0007.

- Агрегат Library: принадлежит Organization, живёт вне проектов, содержит WidgetTypes (позже — ComputationTypes, задача 50, и Themes, задача 51). Хранится в `engineering.*`.
- У Library свой Draft — та же механика, что у Draft проекта: EditLock (задача 31), DraftChange и undo/redo (задача 32), Discard к последнему Library Release.
- Release: Draft Library → новый неизменяемый нумерованный Library Release.
- Draft проекта закрепляет (pin) не более одного Library Release на Library, Library — сколько угодно; добавить/убрать/сменить закрепление — одна DraftChange. Library друг от друга не зависят. Widget ссылается на WidgetType конкретной Library.
- При Deploy (задача 29) содержимое используемых WidgetType из закреплённых релизов копируется в Revision/ProjectFile: Revision не зависит от Library.
- Публичная Library: Organization может открыть Library всем на сервере; любой проект закрепляет её Library Release. Стандартная библиотека GrowSCADA — публичная Library в seed.
- Смена закреплённого релиза: показать, какие Widgets останутся без WidgetType или потеряют порты (как `removeOrphanedPortBindings`).
- UI: Library как отдельный раздел — список библиотек, их WidgetTypes, Release, история релизов; в Project — список закреплённых релизов.
- Миграция: текущие WidgetTypes — в Library стартовой Organization, её первый Library Release закреплён в проекте «Main».
- REST, Bruno, growctl (манифест WidgetType указывает Library), тесты, `CHANGELOG.md`.

## Comments

- 2026-10-04: бывшая задача 18. Заменяет решение Q7 бывшей задачи 36 («Library — часть Draft проекта»): Library отдельна и общая по ссылке на релиз, а не копия в проекте.
