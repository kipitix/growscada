package main

import (
	"log"
	"net/http"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
	"gitverse.ru/kipitix/growscada/internal/interface/ui/root"
)

func main() {
	app.Route("/", func() app.Composer {
		r := &root.Root{}
		r.SetMode(root.ModeOperation)
		return r
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
