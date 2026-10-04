# Checkpoint
Status: ready-for-human
Blocked by: 29, 36

Слой D. Термин — `CONTEXT.md` (Checkpoint).

- Checkpoint = полное операционное состояние проекта на позицию Journal: номер Revision, runtime-сущности, значения/Quality всех тегов.
- Создаётся при каждом Deploy/Rollback и периодически (константа).
- API: состояние на момент T = ближайший Checkpoint до T + записи Journal до T.

## Comments

- 2026-10-04: бывший тикет 09 задачи 36. Решение Q19: Checkpoint на каждый Deploy и периодически.
