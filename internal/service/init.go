package service

import (
	"seventv2tg/internal/config"
	"seventv2tg/internal/service/media"
)

type Services struct {
	Media *media.Converter
}

func New(cfg *config.Config) *Services {
	return &Services{
		Media: media.NewMediaConverter(cfg.Paths.Jobs, cfg.Paths.Result),
	}
}
