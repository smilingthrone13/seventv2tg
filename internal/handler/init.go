package handler

import (
	"seventv2tg/internal/config"
	"seventv2tg/internal/handler/general"
	"seventv2tg/internal/handler/media"
	"seventv2tg/internal/infrastructure/webapi"
	"seventv2tg/internal/service"
)

type Handlers struct {
	General *general.Handler
	Media   *media.Handler
}

func New(cfg *config.Config, apis *webapi.WebAPIs, services *service.Services) *Handlers {
	generalH := general.New(cfg, apis.TgBot)
	mediaH := media.New(cfg, apis, services)

	handlers := &Handlers{
		General: generalH,
		Media:   mediaH,
	}

	return handlers
}
