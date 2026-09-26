# Player и режим Operation

Type: task
Blocked by: 02, 03

- Player на клиенте: начальное состояние (Revision + runtime-сущности + значения тегов на позицию Journal) + применение записей Journal.
- Operation — Player, закреплённый на «сейчас» (записи по SSE); рендер Scenes текущей Revision с живыми значениями.
- Реализация Player не должна знать, живой поток или перемотка — это нужно для задачи History.
