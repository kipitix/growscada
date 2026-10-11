# Player и режим Operation
Status: ready-for-human
Blocked by: 35, 42

Слой D. Термин — `CONTEXT.md` (Player).

- Player на клиенте: начальное состояние (Revision + runtime-сущности + значения тегов на позицию Journal) + применение записей Journal.
- Operation — Player, закреплённый на «сейчас» (записи по SSE); рендер Scenes текущей Revision с живыми значениями.
- Реализация Player не должна знать, живой поток или перемотка — это нужно для задачи 45 (History).
- Временный Operation поверх текущих таблиц — задача 16: заменить источник данных на Revision + Journal, переиспользовать её компонент «Scene с живыми значениями».
- Рекомендация (CQRS): начальное состояние Player отдавать отдельной моделью чтения, а не через агрегаты Draft. Один запрос — Scenes текущей Revision с Widgets, их WidgetTypes и значениями Tag на позицию Journal, в форме, нужной Player. Сейчас Operation собирает это из четырёх запросов (`/widget-types`, `/scenes`, `/scenes/{id}/widgets`, `/tags`). Revision (jsonb) и Journal — уже модели чтения Runtime node; поверх них нужен query-слой (задача 58, ступень 1), а не репозитории агрегатов. Проекции с eventual consistency (ступень 2) не нужны, пока чтение не станет узким местом.

## Comments

- 2026-10-04: бывший тикет 08 задачи 36. Решение Q18: Operation и History — один Player.
- 2026-10-11 (grilling задачи 21): добавлена рекомендация по CQRS — начальное состояние Player через query-слой задачи 58.
