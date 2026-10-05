# Player и режим Operation
Status: ready-for-human
Blocked by: 35, 42

Слой D. Термин — `CONTEXT.md` (Player).

- Player на клиенте: начальное состояние (Revision + runtime-сущности + значения тегов на позицию Journal) + применение записей Journal.
- Operation — Player, закреплённый на «сейчас» (записи по SSE); рендер Scenes текущей Revision с живыми значениями.
- Реализация Player не должна знать, живой поток или перемотка — это нужно для задачи 45 (History).
- Временный Operation поверх текущих таблиц — задача 16: заменить источник данных на Revision + Journal, переиспользовать её компонент «Scene с живыми значениями».

## Comments

- 2026-10-04: бывший тикет 08 задачи 36. Решение Q18: Operation и History — один Player.
