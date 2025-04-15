package handler

import (
	"seventv2tg/internal/config"
	"seventv2tg/internal/handler/media"
	"seventv2tg/internal/infrastructure/webapi"
	"seventv2tg/internal/service"
)

type Handlers struct {
	Media *media.Handler
}

func New(cfg *config.Config, apis *webapi.WebAPIs, services *service.Services) *Handlers {
	mediaH := media.New(cfg, apis, services)

	handlers := &Handlers{
		Media: mediaH,
	}

	return handlers
}
