package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello, this is the GrowScada server!")
	})

	fmt.Println("Server is starting on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
