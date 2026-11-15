# 88_3 — growctl и внутренние имена UI: saved state
Status: ready-for-agent
Priority: 1
Blocked by: 88_2

Третья часть цепочки `88_saved_state_rename.md`; таблица соответствия — там. API и Bruno уже переименованы в 88_2, здесь — пользовательская поверхность growctl и внутренние имена UI.

## Объём

- growctl: команда `commits` → `saved-states` (и `--library`), флаг `deploy --commit N` → `--saved-state N`, `label assign --commit N` → `--saved-state N`, колонка `COMMIT` → `SAVED STATE`, help-тексты и ошибки; `spec.libraryImports` не меняется (значение — номер); вывод `apply`/`import` называет saved state.
- UI: `ui/project/commits.go` → `saved_states.go`, `ui/library/commits.go` → `saved_states.go`; поля `commits`/`commitsLoaded` → `savedStates`/`savedStatesLoaded`, `renderCommitBar` → `renderSavedStateBar`, `data-commit-*` → `data-saved-state-*`; `ui/savedstates` сверить с новыми типами; `eventlog` и `uitest`.
- Тесты growctl (`manifest_test.go`, `growctl_e2e_test.go`, `cli`-тексты) и UI (`uitest`) — новые имена команд, флагов, колонок и атрибутов.
- `tests/device_simulator` и `internal/devicesim` не содержат сохранённых состояний — не трогать.
- Браузерные тексты уже говорят saved state; проверить, что новых «Commit» не появилось.

## Проверка

- `make build`, `make test`, `make bruno_test`.
- growctl e2e: `save` → `saved-states`, `deploy --saved-state N`, `label assign --saved-state N`.
- PWA: History, Deploy, Import, Save.
- `grep -rn -iE "\bcommits?\b"` по `internal/growctl` и `internal/server/interface/ui` находит только историю формата.

## Связанное

- Спецификация — `88_saved_state_rename.md`.
