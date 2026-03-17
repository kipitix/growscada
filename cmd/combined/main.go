package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// hello — компонент главной страницы.
type hello struct {
	app.Compo
	items   []string // данные для отображения
	loading bool     // флаг загрузки
}

// Render отрисовывает компонент.
func (h *hello) Render() app.UI {
	return app.Div().Body(
		app.H1().Text("Пример запроса к серверу"),
		app.Button().
			Text("Загрузить данные").
			OnClick(h.loadData),
		app.If(h.loading, func() app.UI {
			return app.Div().Text("Загрузка...")
		}),
		// Прокручиваемый список
		app.Div().
			Style("height", "200px").
			Style("overflow-y", "scroll").
			Style("border", "1px solid #ccc").
			Style("margin-top", "10px").
			Body(
				app.Range(h.items).Slice(func(i int) app.UI {
					return app.Div().
						Style("padding", "4px").
						Body(
							app.Text(h.items[i]),
						)
				}),
			),
	)
}

// loadData вызывается при клике на кнопку.
func (h *hello) loadData(ctx app.Context, e app.Event) {
	h.loading = true
	h.items = nil
	ctx.Update() // сразу показываем сообщение о загрузке

	// Запускаем HTTP-запрос в отдельной горутине, чтобы не блокировать UI
	go func() {
		resp, err := http.Get("/api/data")
		if err != nil {
			ctx.Update() // обновим UI с ошибкой
			h.loading = false
			h.items = []string{"Ошибка соединения: " + err.Error()}
			ctx.Update()
			return
		}
		defer resp.Body.Close()

		var data []string
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			h.loading = false
			h.items = []string{"Ошибка разбора ответа"}
			ctx.Update()
			return
		}

		h.loading = false
		h.items = data
		ctx.Update() // обновляем список
	}()
}

func main() {
	// Регистрируем компонент для пути "/"
	app.Route("/", func() app.Composer {
		return &hello{}
	})

	// Запускаем приложение в браузере (для WebAssembly)
	app.RunWhenOnBrowser()

	// Настраиваем HTTP-обработчики для сервера
	http.Handle("/", &app.Handler{
		Name:        "Demo",
		Description: "Кнопка со списком",
	})

	// Эндпоинт, возвращающий тестовые данные
	http.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		data := []string{
			"Элемент 1",
			"Элемент 2",
			"Элемент 3",
			"Элемент 4",
			"Элемент 5",
			"Элемент 6",
			"Элемент 7",
			"Элемент 8",
			"Элемент 9",
			"Элемент 10",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	})

	// Запускаем сервер на порту 8080
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
