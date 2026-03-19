package main

import (
	"log"
	"net/http"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

type root struct {
	app.Compo
}

func (h *root) Render() app.UI {
	return app.Div().Body(
		app.H1().Text("Пример запроса к серверу"),
	)
}

func main() {
	// Регистрируем компонент для пути "/"
	app.Route("/", func() app.Composer {
		return &root{}
	})

	// Запускаем приложение в браузере (для WebAssembly)
	app.RunWhenOnBrowser()

	// Настраиваем HTTP-обработчики для сервера
	http.Handle("/", &app.Handler{
		Name:        "GrowSCADA",
		Description: "SCADA to Go",
	})

	// Запускаем сервер на порту 8080
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
