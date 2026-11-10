# Кнопки вне `uiutil.Button`: колонка WidgetTypes и сборщики Project

Status: needs-triage

Найдено при ревью задачи 77: задача собрала палитру и выключенное состояние
кнопок в `uiutil.Button` (`internal/server/interface/ui/uiutil/button.go`) для
страницы выбора и режима Library, но копии тех же палитр и метрик остались в
двух местах, которые грилинг 77 сознательно не трогал.

## Где дубли

- `internal/server/interface/ui/library/render_list.go:46-89` —
  `renderListButtons`: `Delete` и `Create` повторяют `uiutil.ButtonSmall`
  один в один (`flex: 1`, `padding: 4px 0`, `font-size: 13px`,
  `opacity: 0.4`, палитры `--error-*` и `--accent-*`), включая ручное
  выключение. Пакет тот же, что импортирует `uiutil`, — повода держать копию
  нет.
- `internal/server/interface/ui/project/` — около тридцати `app.Button()` и
  шесть сборщиков из решения Q77.1: `deployButton` (`deploy.go:336`, метрика
  `2px 9px`/`12px`, уже с `EventScope(label)`), `projectFileButton`
  (`projectfile.go:376,408`), `draftChangeButton` (`draft_changes.go:163`),
  `releaseCommitButton`, `downloadReleaseButton` (`releases.go:444-500`) и
  `downloadCommitButton` (`commits.go:465`) — последние четыре с метрикой
  `1px 7px`/`11px`. Палитра `--accent-*`/`--error-*`/нейтраль и выключенное
  состояние повторяются в каждом. Отдельно: `tags_render.go:48,72` —
  ещё одна пара Create/Delete с метрикой `ButtonSmall`.

## Что меняется

- `uiutil.Button` получает недостающий пресет под метрику строк таблиц
  (`1px 7px`/`11px`, назвать, например, `ButtonRow`) — рядом с `ButtonPage`,
  `ButtonBar`, `ButtonSmall`, с той же палитрой и тем же выключением.
- `ui/library/render_list.go`: `Delete` и `Create` — тонкие обёртки над
  пресетом small; диалог подтверждения удаления и порядок кнопок в строке
  (`Create`, `Delete`) сохраняются; `data-*`-атрибутов у них нет, но если
  появятся — сохранить.
- `ui/project`: шесть сборщиков становятся тонкими обёртками над
  `uiutil.Button` с пресетом bar/row; их `data-*`-атрибуты, подписи, `Title`
  и `EventScope` остаются как есть (`deployButton` уже разводит обработчики
  по подписи — не потерять). Строковые кнопки `tags_render.go` переводятся на
  пресет small.
- Остальные `app.Button()` в `ui/project` и `ui/library`, которые повторяют
  палитру, переводятся туда же; те, что действительно особые (например кнопка
  подтверждения с собственной вёрсткой), остаются с комментарием, почему они
  не через `uiutil.Button`.
- После уборки комментарий `uiutil.Button` («the Project mode's builders still
  carry their own copies of both») становится правдой наоборот — обновить его.

## Приёмка

1. Внешний вид не меняется ни на пиксель: метрики, палитра, opacity
   выключенной кнопки и порядок кнопок прежние — проверить в браузере до и
   после (шапка Project, шапка Library, страницы выбора, диалоги, колонка
   WidgetTypes, колонка Tags).
2. `app.Button()` в `ui/project` и `ui/library` либо уходит, либо остаётся
   только там, где кнопка действительно не действие общего вида, — с
   комментарием-обоснованием; в репозитории не остаётся второй копии палитры
   и правила «выключенная = `opacity` + `cursor: default`».
3. Поведение обработчиков не меняется: `EventScope` у строковых кнопок
   сохранён там, где был, и добавлен там, где кнопка относится к строке (это
   же свойство go-app чинит задача 79 для шапки Library).
4. `make build`, `make test` зелёные; `make bruno_test` — если правка
   затронула видимые сценарии UI (правило 9); браузерная проверка обоих
   редакторов. REST и контракт не меняются.

## Связанное

- Задача 77 — общий `uiutil.Button` и решения Q77.1 (шесть сборщиков Project
  отложены явно).
- Задача 79 — перепутанные обработчики шапки Library (та же природа go-app).
- Задача 64 — уборка кода Library из старого backlog.

## Comments

- 2026-11-10: заведено из итогового отчёта `/kx_auto` по прогону задач 74–78
  (находка Spec P1 ревью задачи 77).
- 2026-11-10: грилинг-вопрос по конфликту приёмок. Приёмка 1 («вид не
  меняется ни на пиксель») и приёмка 2 («не остаётся второй копии палитры и
  правила выключенной кнопки») противоречили друг другу: кнопки Project
  держали нейтральный фон `--bg` вместо `--bg-hover`, а выключенные не
  приглушались и оставляли `cursor: pointer`. Решение пользователя —
  **полностью унифицировать**: фон `--bg-hover` и `opacity` 0.4 /
  `cursor: default` у всех. Метрики, палитра `accent`/`error` и порядок
  кнопок при этом сохранены; вид включённых кнопок Project стал общим с
  Library, выключенные впервые приглушаются.
- 2026-11-10: сделано. У `uiutil.Button` появился пресет `ButtonRow`
  (`1px 7px`, `11px`, выключенная `0.4`), функция возвращает
  `app.HTMLButton`, чтобы навешивать `data-*`-атрибуты. Через пресеты теперь
  идут: шесть сборщиков Project (`deployButton`, `projectFileButton`,
  `draftChangeButton` — `ButtonBar`; `releaseCommitButton`,
  `downloadReleaseButton`, `downloadCommitButton` — `ButtonRow`), строки
  `tags_render.go` и `library/render_list.go` (`ButtonSmall`), кнопки строк
  Project (`Deploy`, `Project file`, `Edit note`, `Change`, `Rename`,
  `Download` Deployment, `✕` пина и порта) — новый `rowButton` в
  `ui/project/buttons.go`, кнопки редактора Library (`✕`, `+ Add Port`,
  `Apply`). Строковым кнопкам добавлен `app.EventScope` там, где его не было
  (`Deploy`/`Project file`/`Edit note` Release, `Rename`/`Project file`
  Commit, `Download` Deployment, `✕` порта) — по тому же свойству go-app,
  что чинит задача 79.
- 2026-11-10: что осталось осознанно вне `uiutil.Button` — `ui/project/buttons.go`
  документирует одним комментарием: кнопки подвала диалогов (`4px 12px`/
  `13px`, у основной — сплошной `--accent`), иконка сброса поля свойства,
  голые встроенные текстовые действия (`Rename`/`Note` Library) и label
  выбора файла (кнопка не умеет открывать системный пикер). Это не копии
  палитры, а собственные вёрстки; копий палитры и правила
  `opacity`/`cursor: default` вне `uiutil` больше нет.
- 2026-11-10: проверки — `make build`, `make test`, `make bruno_test`
  (коллекции прошли), ручной обход PWA: шапки Project и Library, страница
  выбора, диалоги Save/History/Releases/Deployments/Project file, строки
  Release (Deploy → Running), Commit (Rename, Make Release) и Deployment
  (Download), колонки WidgetTypes и Tags; консоль без ошибок задачи.
  REST, контракт и схемы не менялись.
