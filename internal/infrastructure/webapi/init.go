package webapi

import (
	"seventv2tg/internal/config"
	"seventv2tg/internal/infrastructure/webapi/seventv"
	"seventv2tg/internal/infrastructure/webapi/tgbot"
)

type WebAPIs struct {
	Bot     *tgbot.API
	SevenTV *seventv.API
}

func New(cfg *config.Config) *WebAPIs {
	return &WebAPIs{
		Bot:     tgbot.New(cfg.Debug, cfg.BotApiKey),
		SevenTV: seventv.New(cfg.Paths.Input),
	}
}
