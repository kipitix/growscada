# Проза в коде: Commit → saved state
Status: ready-for-agent
Priority: 1

Найдено при сверке терминологии 2026-11-15. В задачах и документации термин уже приведён к **saved state**; в коде идентификаторы `Commit` остаются до задачи 88 (полное переименование), но комментарии и help-строки говорят «Commit» в прозе.

Задача идёт первой в цепочке очистки: после неё задача 88 меняет только идентификаторы, а не формулировки.

## Что сделать

- В комментариях, doc-комментариях и help-строках `internal/**`, `cmd/**` и `contract/**` писать **saved state** вместо Commit:
  - «a Commit is immutable» → «a saved state is immutable»;
  - «resets the Draft to its last Commit» → «…to its last saved state»;
  - «the number of the Commit» → «the number of the saved state»;
  - «every Commit of a list» → «every saved state of a list»;
  - «commit 0 previews removing the import» → «saved state 0 previews removing the import».
- Пользовательские строки, которые ещё говорят Commit, перевести на saved state: help growctl (`internal/growctl/*.go`), описания методов `apiclient`, тексты UI, если найдутся.
- **Идентификаторы не менять** — это задача 88: `draft.Commit`, `CommitContent`, `CommitService`, `/commits`, `commit_number`, `commit_*` события, таблицы/колонки `commits`/`commit_labels`/`commit_id`, CLI-команда `commits`, флаг `--commit`, колонка `COMMIT`. В комментарии они остаются как есть (`CommitService`, `FindLastCommit`), меняется только проза вокруг них.
- Не трогать «commit» в смысле транзакции БД или git (`after commit`, `racing commits`, `git commit`, `commit the transaction`), `EntryTypeRevision` и комментарии о старых именах Journal/событий — это история формата.
- Задача 87 механическая: поведение, API и схемы не меняются, Bruno и миграции не трогаются.
- Тесты не меняются, кроме случаев, где тест проверяет точный текст пользовательской строки (help growctl) — тогда ожидание обновляется.

## Как проверять

- `make build` и `make test` проходят.
- `grep -rn -iE "//.*\\bcommits?\\b"` по не-тестовому коду: остались только идентификаторы, история формата и «commit» про транзакцию.
- `git diff` не содержит изменений кода — только комментарии и строки.

## Связанное

- Задача 86 — остатки Release в коде (имена, мёртвый `UpsertTagDefinitions`).
- Задача 88 — переименование самих идентификаторов `Commit` → `SavedState`; идёт после этой.
