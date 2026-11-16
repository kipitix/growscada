# Удаление saved state

Status: needs-triage
Priority: 3

Удаление сохранённого состояния Draft — и у Project, и у Library. Сейчас
сохранения только создаются, переименовываются и загружаются обратно; удалить
ошибочное или устаревшее нельзя.

## Что нужно решить до реализации

- **Ссылки на сохранение.** На saved state ссылаются: Deployment
  (`saved_state_number`), импорт Library в Draft проекта (`library_imports`) и
  строка экрана сохранений. Запрещать удаление при ссылке (409 с понятным
  `problem+json`) или разрешать с предупреждением, как Discard теряет импорты?
  Решение записать в ADR 0021.
- **Последнее сохранение.** Если удаляют последнее (`last_saved_state_number`),
  что становится последним и что происходит с `last_saved_state_draft_version`
  и Clean Version? Нумерация не переиспользуется; `latest_saved_state` в ответах
  Library/Project должен уменьшиться.
- **Метки.** `saved_state_labels` удаляются каскадом; `assigned_count` в
  `GET /labels` перечитывается. Метки — не контент, Version Draft не меняется
  (ADR 0025).
- **История и UI.** Строка сохранения получает удаление с подтверждением
  (имя/номер в вопросе); после ответа — тост, перечитывание истории, состояния
  Draft и списка сущностей. Кнопка недоступна, пока идёт Load/Deploy. Экран
  saved states не должен предлагать удалить Draft-строку — это не сохранение.

## Объём

- Домен `draft.Repository`: удаление saved state своей транзакцией, с событием
  `saved_state_deleted` (номер и scope), без Version Draft и DraftChange.
- Application: `SavedStateService.Delete`; проверки ссылок и последнего
  сохранения — здесь, под блокировкой строки Draft.
- REST: `DELETE …/projects/{id}/saved-states/{n}` и то же у Library; 204,
  404 `saved_state_not_found`, 409 при ссылке/запрете. Новый тип события —
  MINOR `api/v0`, `make schemas`; Bruno — запрос с удалением в конце
  последовательности.
- UI: кнопка удаления в строке `ui/savedstates`, подтверждение, обработка
  отказа, перечитывание истории/лейблов/бейджа.
- Тесты: домен, репозиторий, application, REST, UI, Bruno; ручной обход PWA.
- Документы: ADR 0021 (удаление и ссылки), `docs/architecture/*`,
  `GLOSSARY.md` при новом термине, `CHANGELOG.md`.

## Связанное

- Задача 91 — экран saved states и его строка Draft.
- ADR 0021 — saved state как неизменяемый снимок Draft; ADR 0016 — Clean
  Version и Version Draft; ADR 0025 — метки; ADR 0014 — Deployment.

## Comments

- 2026-11-16: заведено по просьбе пользователя, реализация отложена.
