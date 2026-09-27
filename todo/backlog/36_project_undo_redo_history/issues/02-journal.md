# Толстые события и Journal

Type: task
Blocked by: 01

- Операционные события несут полное новое состояние (payload) того, что изменилось: значение/Quality тега, runtime-сущность, Deploy.
- Таблица `runtime.journal(seq bigserial, ts, type, aggregate_id, payload jsonb, schema_version)` — append-only; запись в одной транзакции с изменением состояния.
- SSE отдаёт записи Journal с `seq`; клиент может переподключиться с `Last-Event-ID` и дочитать пропущенное.
- Правки Draft в Journal не пишутся (у них свои события для обновления UI Project).
- Поглощает задачу 24.
