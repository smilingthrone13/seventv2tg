package main

import (
	"log"

	"seventv2tg/internal/app"
	"seventv2tg/internal/config"
)

func main() {
	cfg, err := config.NewConfig("./config")
	if err != nil {
		log.Fatal(err)
	}

	app.New(cfg).Run()
}
