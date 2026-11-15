# Проза в коде: Commit → saved state
Status: ready-for-agent
Priority: 3

Найдено при сверке терминологии 2026-11-15. В задачах и документации термин уже приведён к **saved state**; в коде идентификаторы `Commit` остаются до задачи 88 (полное переименование), но комментарии и help-строки говорят «Commit» в прозе.

## Что сделать

- В комментариях, doc-комментариях и help-строках `internal/**`, `cmd/**` и `contract/**` писать **saved state** вместо Commit: «a saved state is immutable», «resets the Draft to its last saved state», «the number of the saved state» и т. п.
- Пользовательские строки, которые ещё говорят Commit, перевести на saved state (help growctl, описания `apiclient`, тексты UI, если найдутся).
- **Идентификаторы не менять** — это задача 88: `draft.Commit`, `CommitContent`, `/commits`, `commit_number`, `commit_*` события, колонки и таблицы `commits`/`commit_labels`, CLI-команда `commits`, флаг `--commit`, колонка `COMMIT`.
- Не трогать «commit» в смысле транзакции БД или git (`after commit`, `racing commits`, `git commit`), `EntryTypeRevision` и комментарии о старых именах Journal/событий — это история формата.
- Задача 87 механическая: поведение, API и схемы не меняются, Bruno и миграции не трогаются.

## Проверка

- `make build` и `make test` проходят.
- Поиск `//.*Commit` по не-тестовому коду находит только идентификаторы, историю формата и «commit» про транзакцию.

## Связанное

- Задача 86 — остатки Release в коде (имена, мёртвый `UpsertTagDefinitions`).
- Задача 88 — переименование самих идентификаторов `Commit` → `SavedState`; если 88 пойдёт первой, эта задача вливается в неё.
