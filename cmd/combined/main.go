package main

import (
	"log"
	"net/http"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
	"gitverse.ru/kipitix/growscada/internal/application/client/root"
)

func main() {
	app.Route("/", func() app.Composer {
		return &root.Root{}
	})

	app.RunWhenOnBrowser()

	http.Handle("/", &app.Handler{
		Name:        "GrowSCADA",
		Description: "SCADA to Go",
	})

	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
