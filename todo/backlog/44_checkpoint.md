# Checkpoint
Status: ready-for-human
Priority: 3
Blocked by: 35, 42, 72_4

Слой D. Термин — `GLOSSARY.md` (Checkpoint).

- Checkpoint = полное операционное состояние проекта на позицию Journal: номер работающей Deployment (и номер её Release, если есть), runtime-сущности, значения/Quality всех тегов.
- Создаётся при каждом Deploy Release, Rollback и периодически (константа).
- API: состояние на момент T = ближайший Checkpoint до T + записи Journal до T.

## Comments

- 2026-10-04: бывший тикет 09 задачи 36. Решение Q19: Checkpoint на каждый Deploy и периодически.
- 2026-11-07: Deploy теперь активирует Release (72_4); Checkpoint фиксирует Deployment и номер Release. Заблокирован 72_4.
