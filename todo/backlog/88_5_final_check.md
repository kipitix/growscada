# 88_5 — финальная проверка переименования
Status: ready-for-agent
Priority: 1
Blocked by: 88_4

Часть цепочки `88_saved_state_rename.md`.

## Объём

- Поиск остатков: `grep -rniE "\bcommits?\b"` по `internal/`, `cmd/`, `contract/`, `tests/`, `schemas/`, `Makefile`, `GLOSSARY.md`, `AGENTS.md`, `docs/architecture`; каждый остаток — либо транзакция/git, либо история формата (`EntryTypeRevision`, retired event types, имена миграций), либо 410-заглушка.
- `grep -rniE "release|revision|editlock|pins"` — проверить, что задача 86 и 87 закрыты и остались только история/заглушки.
- Полный прогон: `make build`, `make test`, `make schemas` (схемы совпадают), `make bruno_test`.
- Браузер (`chrome-devtools`): Save, History (лейблы, Load), Deploy saved state, Import libraries, Deployments, Operation — консоль без ошибок.
- Проверить старый клиент: `GET /api/v0/projects/{id}/commits` → 410 с новым путём; старый ProjectFile с `library_imports[].commit` → 400.
- Проверить миграцию на данных: имя сохранённого состояния, импорт Library и работающая Deployment после переименования таблиц/колонок.

## Проверка

- Всё выше зелёное; отчёт — что показали браузер и Bruno.

## Связанное

- Спецификация — `88_saved_state_rename.md`; задачи 86, 87.
