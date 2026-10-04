# Rollback и список Revisions
Status: ready-for-human
Blocked by: 29

Слой B.

- Список Revisions проекта на Runtime node (номер, время, откуда пришла).
- Rollback: выбранная прежняя Revision снова становится рабочей как новая Revision с тем же содержимым; история не переписывается. Запись в Journal — в задаче 36.
- Работает без Engineering node.
- Device Tokens Rollback не затрагивает: токен Device, вернувшегося в Revision, снова работает (задача 28).

## Comments

- 2026-10-04: бывший тикет 07 задачи 36. Решение Q30: Rollback на Runtime node создаёт новую Revision со старым содержимым.
