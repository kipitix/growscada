# Test Data for Tests and Debugging

В этой директории содержатся миграции с данными для тестов.

Это цепочка goose в таблице `goose_db_version`, как и `migrations/public`: если туда добавляются миграции, дату миграций здесь нужно переименовывать, чтобы они были позже последней миграции `public`. Таблицы ролей — со схемой (`engineering.tags`, `runtime.tags`, ADR 0011).
