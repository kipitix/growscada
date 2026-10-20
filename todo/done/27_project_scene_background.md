# Показывать фон Scene на холсте Project
Status: ready-for-agent

Появилось при проверке задачи 16. Operation рисует под виджетами `background_html` Scene, а холст Project его не показывает: там однотонный `var(--surface)` с точечной сеткой редактора. Поэтому Engineer не видит фон, который увидит Operator (в seed-данных — тёмно-синие SVG `#1a1a2e` и `#16213e`), и расставляет виджеты вслепую.

- На холсте Project (`project/render_scene.go`, `renderSceneCanvas`) под виджетами рендерить `background_html` выбранной Scene так же, как в Operation (`livescene.View.renderBackground`): iframe без скриптов (`sandbox="allow-same-origin"`: пустой `sandbox=""` go-app выбрасывает — см. ниже), `pointer-events: none`, `overflow: hidden` в документе фона. Пустой `background_html` — как сейчас, однотонный фон. Код рендера фона вынести в общий хелпер `uiutil.SceneBackground(html string) app.UI` (отдельный файл в `uiutil`); `livescene.View` тоже переходит на него, `renderBackground` удаляется.
- Точечная сетка остаётся поверх фона: отдельный слой с `pointer-events: none` между фоном и виджетами, чтобы сетка была видна на любом фоне и не мешала drag-and-drop и выделению. Порядок слоёв по DOM: однотонный `var(--surface)` на самом холсте → iframe фона (если `background_html` не пуст) → div сетки (`position: absolute; inset: 0`) → виджеты. Обработчики drop/click/mousedown остаются на холсте (`currentTarget` в `OnDrop` — по-прежнему холст). Контраст сетки (`--dot-color`) не меняем — это задача 41.
- Правка `background_html` в панели свойств Scene видна на холсте после сохранения (blur textarea, как сейчас; кнопку Apply не добавляем, превью при наборе не делаем — холст рисует сохранённое `sc.BackgroundHTML`, обновлённое оптимистично), без перезагрузки страницы; изменения из другой вкладки или `growctl` подхватываются живой перезагрузкой, как остальные свойства Scene.
- Фон не должен перехватывать события мыши: перетаскивание типа виджета на холст (drop), клик по пустому месту (снятие выделения), drag/resize/rotate виджетов работают как раньше.

## Тесты

- Unit-тест `uiutil.SceneBackground`: `sandbox=""`, `pointer-events: none`, HTML фона в srcdoc.
- UI-тест Project (`TestEngine` + `servertest.StartAPI`): у Scene с `background_html` на холсте есть iframe фона под слоем сетки; у Scene с пустым фоном — нет.
- Тест `livescene`: Operation по-прежнему рисует фон (через хелпер).
- Взаимодействия мышью — ручная проверка в браузере.

## Приёмка

- `make full_restart`: в Project на вкладке «Main Dashboard» холст тёмно-синий, как в Operation, сетка видна поверх.
- Изменить `background_html` Scene в Project: новый фон появляется на холсте Project и в Operation (другая вкладка).
- Создание, перемещение, поворот и выделение виджетов на холсте работают как до изменения.
- Проверить в браузере через chrome-devtools; `make build` и `make test` проходят.

## Comments

- 2026-10-20 (grill): «Apply» — сохранение по blur textarea, кнопку не добавляем; хелпер — `uiutil.SceneBackground`; сетка — отдельный слой между фоном и виджетами; контраст сетки — задача 41; глоссарий и ADR не меняются.
- 2026-10-20 (реализация): go-app (`attributes.Set`) не хранит атрибут с пустым значением, поэтому `Attr("sandbox", "")` не попадал в DOM — фон в Operation рендерился без sandbox, и скрипты в `background_html` выполнялись. Фон теперь в `sandbox="allow-same-origin"`: без `allow-scripts` скрипты не выполняются (проверено в браузере: `<script>` в фоне не меняет страницу).
- 2026-10-20 (ревью): по итогам код-ревью реализация отошла от спецификации выше.
  - `uiutil.SceneBackground` — компонент `&uiutil.SceneBackground{HTML: …}`, а не функция. Рендерится всегда, при пустом `HTML` скрыт (`display: none`): go-app сопоставляет детей по позиции, и появление или исчезновение слоя фона сдвигало компоненты виджетов и перезагружало их iframe.
  - Песочница — `sandbox=""` (полная, непрозрачный origin) вместо `allow-same-origin`, который давал фону origin приложения. Атрибут ставится `setAttribute` в `OnMount`/`OnUpdate` до документа: в `Render` пустое значение go-app выбросил бы. Документ задаётся свойством `srcdoc` через `uiutil.IframeSrcdoc` и пересобирается только при смене `HTML`. Проверено в браузере: `sandbox=""`, `contentDocument` недоступен, при смене Scene тот же iframe получает новый фон.
  - У iframe фона `color-scheme: light` и прозрачный фон, как у `WidgetIFrame`: иначе в тёмной теме непокрытая фоном часть Scene была бы белой. `tabindex="-1"` и `aria-hidden="true"` убирают фон из обхода Tab и от скринридеров. В документе фона `html,body{height:100%}`, чтобы фон в процентах заполнял Scene.
  - Виджеты лежат в своём слое `uiutil.SceneWidgetLayer` (`isolation: isolate`). В Project, как и в Operation, у виджета `z-index: Position.Z`; отрицательный Z не уводит виджет под фон.
  - Контраст сетки записан требованием в задачу 41. Обрезка выступающих за край Scene виджетов в Project (в Operation они обрезаются) не сделана.
