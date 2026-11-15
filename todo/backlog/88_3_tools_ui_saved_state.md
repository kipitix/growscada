# 88_3 — growctl, UI-код и Bruno: saved state
Status: needs-triage
Priority: 3
Blocked by: 88_2

Часть цепочки `88_saved_state_rename.md`; таблица соответствия — там.

## Объём

- growctl: команда `commits` → `saved-states` (и `--library`), флаг `deploy --commit N` → `--saved-state N`, `label assign --commit N` → `--saved-state N`, колонка `COMMIT` → `SAVED STATE`, help-тексты и ошибки; `spec.libraryImports` не меняется (значение — номер); вывод `apply`/`import` называет saved state.
- UI: `ui/project/commits.go` → `saved_states.go`, `ui/library/commits.go` → `saved_states.go`, `renderCommitBar`, `data-commit-*` → `data-saved-state-*`, поля состояния `commits`/`commitsLoaded` → `savedStates`/`savedStatesLoaded`; `ui/savedstates` уже назван правильно — сверить API-вызовы; `eventlog` и `uitest`.
- Bruno: переименовать файлы запросов и папки, обновить URL/поля; `TAG_NAME`/симулятор не затрагиваются.
- `tests/device_simulator` и `internal/devicesim` не содержат сохранённых состояний — не трогать.
- Браузерные тексты уже говорят saved state; проверить, что новых «Commit» не появилось.

## Проверка

- `make build`, `make test`, `make bruno_test`.
- growctl e2e: `save` → `saved-states`, `deploy --saved-state N`, `label assign --saved-state N`.
- PWA: History, Deploy, Import, Save.

## Связанное

- Спецификация — `88_saved_state_rename.md`.
