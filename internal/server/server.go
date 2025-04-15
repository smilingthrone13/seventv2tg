package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"seventv2tg/internal/config"
	"seventv2tg/internal/handler"
)

type botApi interface {
	GetUpdatesChan() tgbotapi.UpdatesChannel
	Shutdown()
}

type botAPI interface {
	GetUpdatesChan() tgbotapi.UpdatesChannel
	Shutdown()
}

type (
	InitParams struct {
		Config   *config.Config
		Api      botApi
		Handlers *handler.Handlers
	}
	Server struct {
		cfg      *config.Config
		api      botApi
		handlers *handler.Handlers
	}
)

func New(p *InitParams) *Server {
	return &Server{
		cfg:      p.Config,
		api:      p.Api,
		handlers: p.Handlers,
	}
}

func (s *Server) Start() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	updatesChan := s.api.GetUpdatesChan()

	fmt.Println("Server started!")

	for {
		select {
		case update := <-updatesChan:
			go s.handleUpdate(&update)
		case <-c:
			s.api.Shutdown()

			return
		}
	}
}

func (s *Server) handleUpdate(update *tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	if update.Message.IsCommand() {
		return
	}

	s.handlers.Media.CreateVideoFromEmote(context.Background(), update.Message)
}
