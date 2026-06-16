# Перенос HTML → go-app

Гайд по воссозданию дизайна в `github.com/maxence-charriere/go-app/v10/pkg/app`.
go-app строит DOM из Go-билдеров; CSS подключается как обычный файл без
изменений. Ниже — правила маппинга для **этого** экрана.

## 1. CSS — переносится как есть
Скопируйте весь блок `<style>` из `SCADA Widget Editor.html` в статический файл,
например `web/editor.css`, и подключите:

```go
app.Handler{
    Name:   "GrowSCADA Widget Editor",
    Styles: []string{"/web/editor.css"},
}
```
Ничего в CSS править не нужно — темы/акценты/токены уже на CSS-переменных.
Тема и акцент задаются классами на корневом элементе компонента.

## 2. Маппинг тегов и атрибутов
| HTML | go-app |
|---|---|
| `<div class="panel">` | `app.Div().Class("panel")` |
| `<button class="btn btn-accent btn-sm">` | `app.Button().Class("btn btn-accent btn-sm")` |
| `<span>text</span>` | `app.Span().Text("text")` |
| `<input type="number" value=...>` | `app.Input().Type("number").Value(v)` |
| `style="flex:0 0 224px"` | `.Style("flex", "0 0 224px")` |
| `onClick={...}` | `.OnClick(func(ctx app.Context, e app.Event){...})` |
| `onPointerDown` | `.On("pointerdown", handler)` |
| вложенные дети | `.Body(child1, child2, …)` |
| список (`map`) | `app.Range(items).Slice(func(i int) app.UI {…})` |
| условие (`a ? b : c`) | `app.If(cond, func() app.UI{…}).Else(func() app.UI{…})` |

Inline-SVG иконки: используйте `app.Raw("<svg …>…</svg>")` (go-app вставит как
есть) — это проще, чем строить SVG билдерами. Шаблон виджета в превью тоже
вставляется через `app.Raw(widget.Template)`.

## 3. Скелет компонента
```go
type WidgetEditor struct {
    app.Compo
    theme      string // "dark" | "light"
    accent     string // "teal"
    selID      string
    codeLayout string // "tabs" | "split-st" | "split-sb"
    codeTab    string // "html" | "js"
    splitRatio float64
    values     map[string]map[string]string // widgetID -> port -> raw
}

func (e *WidgetEditor) Render() app.UI {
    w := widgetByID(e.selID)
    return app.Div().
        Class("editor theme-"+e.theme, "acc-"+e.accent).
        Body(
            e.topBar(),
            app.Div().Class("ed-body").Body(
                e.widgetTypes(),
                e.codeColumn(w),
                e.portsAndData(w),
                e.preview(w),
            ),
            e.statusBar(w),
        )
}

func (e *WidgetEditor) toggleTheme(ctx app.Context, _ app.Event) {
    if e.theme == "dark" { e.theme = "light" } else { e.theme = "dark" }
}
```

## 4. Колонка кода — режимы и Apply
**Важно:** группа кнопок-режимов и кнопка **Apply** всегда в DOM и на фиксированных
позициях (правый блок `panel-head-actions`). Не прятать Apply при смене режима —
это убирает «прыжок» вёрстки. Левый блок шапки меняется: сегмент в `tabs`,
заголовок в split.

```go
func (e *WidgetEditor) codeColumn(w Widget) app.UI {
    head := app.Div().Class("panel-head").Style("height","40px").Style("flex-basis","40px").Body(
        app.If(e.codeLayout == "tabs",
            func() app.UI { return e.codeTabsSeg() },
        ).Else(
            func() app.UI { return app.Div().Class("panel-title").Text("Template + Script") },
        ),
        app.Div().Class("panel-head-actions").Body(
            e.modeSwitch(),                 // всегда
            app.Button().Class("btn btn-accent btn-sm").  // всегда, всегда яркая
                Body(icon(checkSVG), app.Text(" Apply")),
        ),
    )
    var body app.UI
    switch e.codeLayout {
    case "tabs":
        body = e.codeBlock(pick(e.codeTab, w)) // template ИЛИ script
    case "split-st":
        body = e.splitView(w.Script, w.Template) // скрипт сверху
    case "split-sb":
        body = e.splitView(w.Template, w.Script) // скрипт снизу
    }
    return app.Div().Class("panel").Style("flex","1.3 1 0").Body(head, app.Div().Class("panel-body").Style("padding","0").Body(body))
}
```

## 5. Подсветка синтаксиса
В макете токенайзеры на JS (`tokenizeHTML`, `tokenizeJS` в `shared.jsx`). В Go
портируйте их логику (простые регэксп-правила → классы `t-*`) и собирайте
`code-gutter` (номера строк) + `code-body` (строки из `<span class="t-…">`).
Правила — в `shared.jsx`, секции `tokenizeJS`/`tokenizeHTML`. Альтернатива:
любой Go-хайлайтер (напр. `alecthomas/chroma`) с кастомным маппингом классов на
те же CSS-переменные `--t-*`. Главное — сохранить имена классов из CSS.

## 6. Перетаскиваемый разделитель (split)
`cresize` — `.On("pointerdown", …)`; в обработчике подписаться на
`pointermove`/`pointerup` (через `app.Window().Call("addEventListener", …)` или
JS-интероп), считать долю `(clientY-top)/height`, зажать в 0.18…0.82, писать в
`e.splitRatio` и `e.Update()`. Высоты тайлов — `.Style("flex-grow", ratio)` и
`(1-ratio)`.

## 7. Real-time данные → превью
Контролы Input Data на `.OnInput`/`.OnChange` пишут raw-строку в `e.values` и
зовут `e.Update()`. Превью: контейнер с `app.Raw(w.Template)`, после монтирования
вызвать `render(el, inputs)` виджета. В проде скрипт виджета исполняется вашим
рантаймом индикации; `inputs` = `coerce(port.Type, raw)` по каждому порту
(boolean←"true", number←parseFloat, string←as-is).

## 8. Чек-лист соответствия
- [ ] Все классы из CSS сохранены 1:1 (вёрстка завязана на них).
- [ ] Корень несёт `theme-* acc-teal`; смена темы = смена класса.
- [ ] Apply одинаковая везде: `btn btn-accent btn-sm` + галочка, всегда видна.
- [ ] Группа режимов + Apply не сдвигаются при переключении раскладки.
- [ ] Нет кнопки Simulate; Input Data применяется в реальном времени (без Apply).
- [ ] Номера строк + подсветка в обоих редакторах кода.
- [ ] Превью на фоне-сетке, карточка 240×240, meta-строка со значениями портов.
- [ ] Status bar окрашен в акцент.
