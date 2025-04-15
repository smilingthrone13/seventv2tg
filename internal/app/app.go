package app

import (
	"log"
	"os"
	"seventv2tg/internal/config"
	"seventv2tg/internal/handler"
	"seventv2tg/internal/infrastructure/webapi"
	"seventv2tg/internal/server"
	"seventv2tg/internal/service"
)

type App struct {
	cfg    *config.Config
	server *server.Server
}

func New(cfg *config.Config) *App {
	webAPI := webapi.New(cfg)

	services := service.New(cfg)

	handlers := handler.New(cfg, webAPI, services)

	s := server.New(
		&server.InitParams{
			Config:   cfg,
			Api:      webAPI.Bot,
			Handlers: handlers,
		},
	)

	app := &App{
		cfg:    cfg,
		server: s,
	}

	err := app.setupDirs()
	if err != nil {
		log.Fatal(err)
	}

	return app
}

func (a *App) Run() {
	a.server.Start()
}

func (a *App) setupDirs() error {
	for _, dir := range []string{a.cfg.Paths.Input, a.cfg.Paths.Jobs, a.cfg.Paths.Result} {
		err := os.RemoveAll(dir)
		if err != nil {
			return err
		}

		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return err
		}
	}

	return nil
}
