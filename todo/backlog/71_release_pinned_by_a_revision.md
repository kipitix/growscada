# Release, названный работающей Revision, не должен пропадать вместе с Library

Status: ready-for-agent

Найдено ревью задачи 35_4 (подозрение, подтверждённое чтением кода; в ветке 35_4 не чинилось — решение затрагивает ADR 0007 и межнодовую проверку).

`Discard` Проекта восстанавливает Draft по работающей Revision: `NewDiscardProjectPolicy.restorePins` находит Release по номеру (`p.release`, `application/discard_project_policy.go`). При этом `NewLibraryCleanupPolicy` удаляет Library вместе с её Releases, как только её никто не пинит (ADR 0007, `AGENTS.md`, «Libraries»). Отпинить Library и удалить её можно, пока Revision Проекта всё ещё называет один из её Releases.

Следствие: `restorePins` получает `library.ErrReleaseNotFound`, job `discard_project` ретраится вечно (PolicyRunner повторяет ошибку до бесконечности), пометка `projects.discarding` не снимается, любая запись Draft отвечает 409 `ErrProjectDiscarding`, отменить Discard нельзя. То же ждёт любой будущий потребитель, который читает Release по номеру из Revision.

## Что решить

- Выбрать одно из двух и записать решение в ADR 0007 (и, если нужно, в ADR 0012):
  - **Защита:** Library (и её Release) не удаляется, пока хотя бы одна Revision какого-либо Project называет её Release. Проверка требует вопроса к Runtime-ноде из cleanup-политики Engineering-ноды (роли, ADR 0011) — порт `RuntimeNode` расширяется запросом вроде «называет ли какая-нибудь Revision этот Release».
  - **Деградация:** Discard не падает, если Release исчез: пин из Revision без Release пропускается с событием/логом, Draft остаётся без этого пина (Draft ≠ Revision, зато система не блокируется). Тогда же решить, что видит `DeployState`/`DeployPreview` после такого Discard.
- Покрыть тестами: Discard после удаления Library (оба сценария из решения) и удаление Library при живой Revision.
- Проверить, что `PolicyRunner` не оставляет вечно ретраящийся job после решения (ключ `project:<id>`, `discard_project`).

## Ссылки

- задача 35, часть 4 (`todo/backlog/35_4_engineering_deploy_discard.md`) — ревью, находка «Discard встаёт навсегда, если Release пина исчез»;
- задача 35, часть 2 (`todo/done/35_2_revision_storage.md`) — `runtime.revisions` хранит ProjectFile с пинами как провенанс.
