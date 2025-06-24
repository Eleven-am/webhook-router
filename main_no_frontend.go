package main

import (
	"embed"
	"log"

	_ "webhook-router/docs"
	"webhook-router/internal/app"
)

// Empty embed for running without frontend

//go:embed docs/swagger.json
var emptyFS embed.FS

func main() {
	if err := app.Run(emptyFS); err != nil {
		log.Fatal(err)
	}
}
