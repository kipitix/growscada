# 88_4 — документы: saved state как единственный термин
Status: ready-for-agent
Priority: 1
Blocked by: 88_3

Четвёртая часть цепочки `88_saved_state_rename.md`.

## Объём

- Новый ADR 0026 «Saved state is the name of a Commit»: решение 2026-11-15, отмена решения задачи 84 оставить Commit в коде, порядок и совместимость; поправка к ADR 0021.
- `GLOSSARY.md`: **Saved state** — единственный термин; **Commit** — прежнее имя, `_Avoid_`. Убрать оговорку «идентификаторы ещё Commit».
- `AGENTS.md`: инварианты и «The words that matter» — saved state; CLI-примеры с `--saved-state`; `draft.SavedState` вместо `draft.Commit`.
- `docs/architecture/{domain,application,infrastructure,interface}.md`: идентификаторы и проза — saved state; исторические миграции и `EntryTypeRevision` остаются как имена формата.
- `CHANGELOG.md`: новая запись (Breaking: пути, поля, события, CLI-команда и флаг переименованы; миграция; старые маршруты — 410; старые ProjectFile — 400).
- ADR 0007/0012/0013/0014/0015/0021 и старые записи CHANGELOG — не переписывать (история).
- `todo/done/` не переписывать: это история задач.

## Проверка

- `grep -rniE "\bcommits?\b"` в `docs/architecture`, `GLOSSARY.md`, `AGENTS.md` находит только историю формата/транзакции.
- `AGENTS.md` остаётся меньше 25 KB; `make test` зелёный.

## Связанное

- Спецификация — `88_saved_state_rename.md`.
