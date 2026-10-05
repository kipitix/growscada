# Journal: запись операционных событий
Status: ready-for-human
Blocked by: 32, 35

Слой D. Термин — `CONTEXT.md` (Journal); архитектура — ADR 0002.

- Операционные события несут полное новое состояние (payload) того, что изменилось: значение/Quality тега, runtime-сущность, Deploy, Rollback.
- Таблица `runtime.journal(project_id, seq bigserial, ts, type, aggregate_id, payload jsonb, schema_version)` — append-only; запись в одной транзакции с изменением состояния. Контракт записи — `contract/record/v0`.
- Записи Deploy (задача 35) и Rollback (задача 36) в Journal.
- SSE отдаёт записи Journal с `seq`; клиент может переподключиться с `Last-Event-ID` и дочитать пропущенное.
- Правки Draft в Journal не пишутся (у них свои события для обновления UI Project).

## Comments

- 2026-09-26: поглотила задачу «Запись и воспроизведение событий» (бывшая 24): здесь запись, воспроизведение — задачи 44–46.
- 2026-10-04: бывший тикет 02 задачи 36. Решения: Q4 (состояние в таблицах + Journal с полным новым состоянием в каждой записи), Q17 (один Journal для всего, включая значения тегов). Запись Deploy/Rollback перенесена сюда, чтобы Deploy не зависел от Journal.
