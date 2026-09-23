# Issue tracker: todo/ markdown

Задачи и спецификации этого репозитория лежат markdown-файлами в `todo/`.

## Conventions

- Открытые задачи: `todo/backlog/NN_snake_case_slug.md`; выполненные переносятся (`git mv`) в `todo/done/` с тем же именем.
- `NN` — сквозной номер: следующий = максимальный номер в `backlog/` и `done/` + 1.
- Файл начинается с заголовка `# <Название по-русски>`, затем тело — список требований (bullet list). Язык — русский.
- Triage-состояние — строка `Status: <label>` сразу под заголовком (см. `triage-labels.md`). Отсутствие строки = `needs-triage`.
- Большая задача, разбитая на тикеты: каталог `todo/backlog/NN_slug/` со `spec.md` и тикетами `issues/NN-slug.md` (нумерация с `01`, по файлу на тикет).
- Комментарии и история обсуждения дописываются в конец файла под `## Comments`.
- Связанные задачи ссылаются друг на друга по номеру (`задача 13`).

## When a skill says "publish to the issue tracker"

Создать новый файл в `todo/backlog/` по правилам выше.

## When a skill says "fetch the relevant ticket"

Прочитать файл по пути или номеру (`todo/backlog/NN_*` либо `todo/done/NN_*`). Спецификация ветки обычно — задача, чей slug совпадает с именем ветки.

## Wayfinding operations

- **Map**: `todo/backlog/NN_<effort>/map.md`.
- **Child ticket**: `todo/backlog/NN_<effort>/issues/NN-<slug>.md`, строки `Type:` (`research`/`prototype`/`grilling`/`task`) и `Status:` (`claimed`/`resolved`).
- **Blocking**: строка `Blocked by: NN, NN`; тикет разблокирован, когда все перечисленные `resolved`.
- **Frontier**: открытые, незаблокированные, не занятые тикеты; первый по номеру.
- **Claim**: `Status: claimed` и сохранить до начала работы.
- **Resolve**: ответ под `## Answer`, `Status: resolved`, краткая ссылка в Decisions-so-far в `map.md`.
