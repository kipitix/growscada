# Убрать дубли UI-кода, оставшиеся после задачи 73

Status: ready-for-agent
Priority: 1

Появилось при ревью задачи 73 (находка Standards 3). Поведение верное, но в
коде остались дубли, которые стоит убрать, пока страница выбора новая.

## Решения грилинга (2026-11-10)

1. **Кнопки — один сборщик в `uiutil` с пресетом вёрстки.** Цветовая палитра
   (`accent`/`error`/`""`) и выключенное состояние продублированы трижды:
   `picker.button` (`padding 3px 10px`, `font 12`, `opacity .5`),
   `ui/library` `barButton` (`2px 9px`, `12`, `.4`) и `smallButton`
   (`flex: 1`, `4px 0`, `13`, `.4`); геометрия у них разная.
   `uiutil.Button(text, kind, enabled, title, layout, onClick, options…)`, где
   `layout` — `bar` / `small` / `page`, хранит точные метрики каждого варианта и
   общую палитру; `picker.button` и `ui/library` `barButton`/`smallButton`
   становятся тонкими обёртками, вызовы не меняются. Шесть сборщиков
   `ui/project` (`deployButton`, `projectFileButton`, `draftChangeButton`,
   `releaseCommitButton`, `downloadReleaseButton`, `downloadCommitButton`) не
   трогаем: у них свои `data-*`-атрибуты и подписи, и это отдельная задача.
2. **`stateColor` и `stateBorder` — одна функция.** Оба `switch` в
   `ui/picker/picker.go` повторяют список состояний; сводятся к одной
   `stateBadge(kind) (color, border string)` (таблица или один `switch` с двумя
   возвратами).
3. **`descriptionOf` — в свой файл пакета `application`.** Сейчас она объявлена
   в `application/project_application_service.go:196` и вызывается из
   `library_application_service.go` (оба файла — один пакет). Переезжает как
   есть в `application/description_input.go`: поведение не меняется, домен и
   контракт не трогаются, «отсутствующее значение сохраняет текущее» остаётся
   семантикой запроса, а не value object'а. В `domain/description` и `appdto` не
   едет: первому это чужое правило, второму — чужое поведение.

## Приёмка

1. Поведение и внешний вид не меняются; существующие UI-тесты зелёные.
2. Новых мест дублирования не появляется: палитра и выключенное состояние
   кнопок — в одном месте, цвета бейджа — в одной функции.
3. `descriptionOf` доступна обоим сервисам из своего файла; `go vet` не ругается
   на неиспользуемые импорты в `project_application_service.go`.
4. `make build`, `make test` зелёные. Bruno не нужен: REST-слой не меняется
   (перенос `descriptionOf` поведения не меняет).

## Связанное

- Задача 73 — источник дублей.
- Задача 76, 78 — соседние находки ревью 73.
- Задача 60, 64 — соседние UI-задачи из старого backlog.

## Comments

- 2026-11-10: заведено по итогам ревью задачи 73; чисто косметическая уборка.
- 2026-11-10: грилинг — общий `uiutil.Button` с пресетами вместо трёх копий
  палитры (Q77.1), слияние `stateColor`/`stateBorder` (решения не требовало),
  `descriptionOf` → `application/description_input.go` (Q77.3).
  Вопрос «английская подпись вместо `← Список`» вынесен в задачу 78: он меняет
  видимый текст, а приёмка 77 — «вид не меняется».
- 2026-11-10: аудит по DDD: расхождений нет — только UI-хелперы и переезд
  функции внутри пакета `application`, границы агрегатов, слои и контракт не
  затрагиваются.
- 2026-11-10: сделано. `uiutil.Button` с тремя пресетами, `picker.button` и
  библиотечные `barButton`/`smallButton` — обёртки, `stateColor`/`stateBorder` →
  `stateBadge`, `descriptionOf` в `application/description_input.go`. Ревью
  нашло одно замечание — комментарий `uiutil.Button` называл себя единственным
  местом палитры, тогда как `ui/project` и `ui/library/render_list.go` держат
  свои копии; формулировка уточнена (сборщики Project отложены решением Q77.1,
  кнопки колонки WidgetTypes грилинг не перечислял — кандидат в отдельную
  задачу). Вне объёма: в режиме Library кнопка History открывает панель
  Releases — прежний дефект позиционного diff go-app (`eventHandler.Equal`
  сравнивает указатель кода замыкания, а у кнопок шапки нет `app.EventScope`);
  воспроизведён и на сборке `path`, в задачу 77 не входит.
- 2026-11-10: проверки — `make build`, `make test`, `make bruno_test` (214/214 и
  6/6); в браузере пройдены страницы выбора Library и Project, шапка и диалоги
  Library (метрики `page`/`bar`/`small`, палитра, выключенное состояние), консоль
  без ошибок задачи.
