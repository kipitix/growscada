# Checkpoint

Type: task
Blocked by: 02, 03

- Checkpoint = полное операционное состояние на позицию Journal: номер Revision, runtime-сущности, значения/Quality всех тегов.
- Создаётся при каждом Deploy/Rollback и периодически (константа).
- API: состояние на момент T = ближайший Checkpoint до T + записи Journal до T.
