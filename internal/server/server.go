package server

import (
	"context"
	"log"
	"os"
	"os/signal"
	"slices"
	"sync"
	"syscall"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"seventv2tg/internal/config"
	"seventv2tg/internal/handler"
)

const (
	startCommand       = "start"
	maintenanceCommand = "maintenance"

	isInMaintenanceMessage = "Bot is currently in maintenance. Try again later."
)

type botApi interface {
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

		isInMaintenance bool
		mu              sync.RWMutex
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

	log.Println("Server started!")
	//log.Printf("Start configuration: debug: %t; admins: %v", s.cfg.Debug, s.cfg.AdminIDs)

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
		switch update.Message.Command() {
		case startCommand:
			s.handlers.General.StartResponse(update.Message.Chat.ID)
		case maintenanceCommand:
			if !slices.Contains(s.cfg.AdminIDs, update.Message.From.ID) {
				return
			}

			status := s.switchMaintenanceStatus()
			s.handlers.General.MaintenanceResponse(update.Message.Chat.ID, status)
			log.Printf("Maintenance status set to %t by user %d\n", status, update.Message.From.ID)
		}

		return
	}

	s.mu.RLock()
	if s.isInMaintenance {
		s.mu.RUnlock()
		s.handlers.General.MessageResponse(update.Message.Chat.ID, isInMaintenanceMessage)

		return
	}
	s.mu.RUnlock()

	s.handlers.Media.CreateVideoFromEmote(context.Background(), update.Message)
}

func (s *Server) switchMaintenanceStatus() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.isInMaintenance = !s.isInMaintenance

	return s.isInMaintenance
}
