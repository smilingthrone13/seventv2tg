package tgbot

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/pkg/errors"
)

type API struct {
	bot *tgbotapi.BotAPI
}

func New(debug bool, apiKey string) *API {
	bot, err := tgbotapi.NewBotAPI(apiKey)
	if err != nil {
		log.Fatalf("Error creating bot: %v", err)
	}

	bot.Debug = debug

	return &API{
		bot: bot,
	}
}

func (b *API) SendMessage(chatID int64, message string) (tgbotapi.Message, error) {
	const errMsg = "BotAPI.SendMessage"

	msg, err := b.bot.Send(tgbotapi.NewMessage(chatID, message))
	if err != nil {
		return tgbotapi.Message{}, errors.Wrap(err, errMsg)
	}

	return msg, nil
}

func (b *API) DeleteMessage(chatID int64, messageID int) error {
	const errMsg = "BotAPI.DeleteMessage"

	msg := tgbotapi.NewDeleteMessage(chatID, messageID)
	_, err := b.bot.Request(msg)

	return errors.Wrap(err, errMsg)
}

func (b *API) SendAttachment(attachment tgbotapi.Chattable) error {
	const errMsg = "BotAPI.SendAttachment"

	_, err := b.bot.Send(attachment)

	return errors.Wrap(err, errMsg)
}

func (b *API) GetUpdatesChan() tgbotapi.UpdatesChannel {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	return b.bot.GetUpdatesChan(u)
}

func (b *API) Shutdown() {
	log.Println("Stopping bot...")

	b.bot.StopReceivingUpdates()
}
