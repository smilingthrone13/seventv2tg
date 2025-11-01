package general

import (
	"fmt"
	"seventv2tg/internal/config"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type (
	botApi interface {
		SendMessage(chatID int64, message string) (tgbotapi.Message, error)
	}

	Handler struct {
		cfg *config.Config
		api botApi
	}
)

func New(cfg *config.Config, botAPI botApi) *Handler {
	return &Handler{
		cfg: cfg,
		api: botAPI,
	}
}

func (h *Handler) StartResponse(chatID int64) {
	message := "Welcome to 7tv2tg bot!\n" +
		"Pick any emote fom https://7tv.app/emotes?a=1 and send me its page link. " +
		"You can send up to 3 links if you want to overlay emotes.\n" +
		"Remember, Telegram restricts animated stickers to 3 seconds max, " +
		"so longer emotes will be cut."

	_, _ = h.api.SendMessage(chatID, message)
}

func (h *Handler) MaintenanceResponse(chatID int64, maintenanceStatus bool) {
	message := fmt.Sprintf("Maintenance status switched to %t", maintenanceStatus)

	_, _ = h.api.SendMessage(chatID, message)
}

func (h *Handler) MessageResponse(chatID int64, message string) {
	_, _ = h.api.SendMessage(chatID, message)
}
