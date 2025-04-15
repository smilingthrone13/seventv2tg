package media

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"

	"seventv2tg/internal/config"
	"seventv2tg/internal/domain"
	"seventv2tg/internal/infrastructure/webapi"
	"seventv2tg/internal/service"
)

const workerCount = 3

type Handler struct {
	cfg      *config.Config
	apis     *webapi.WebAPIs
	services *service.Services

	reqQueue chan domain.UserRequest

	activityCache *cache.Cache
}

func New(cfg *config.Config, apis *webapi.WebAPIs, services *service.Services) *Handler {
	h := &Handler{
		cfg:           cfg,
		apis:          apis,
		services:      services,
		reqQueue:      make(chan domain.UserRequest, 50),
		activityCache: cache.New(cache.NoExpiration, cache.NoExpiration),
	}

	for range workerCount {
		go h.mediaWorker()
	}

	return h
}

func (h *Handler) CreateVideoFromEmote(ctx context.Context, message *tgbotapi.Message) {
	if _, hit := h.activityCache.Get(strconv.FormatInt(message.Chat.ID, 10)); hit {
		_, _ = h.apis.Bot.SendMessage(message.Chat.ID, "You have another emote being processed, please wait")

		return
	}

	emoteID, err := h.validateUserInput(message.Text)
	if err != nil {
		_, _ = h.apis.Bot.SendMessage(message.Chat.ID, "Invalid emote URL")

		return
	}

	h.activityCache.Set(strconv.FormatInt(message.Chat.ID, 10), struct{}{}, cache.NoExpiration)
	defer h.activityCache.Delete(strconv.FormatInt(message.Chat.ID, 10))

	req := domain.UserRequest{
		ChatID:           message.Chat.ID,
		ReplyToMessageID: message.MessageID,
		EmoteID:          emoteID,
		ErrChan:          make(chan error),
	}
	h.reqQueue <- req

	msg, err := h.apis.Bot.SendMessage(req.ChatID, "Emote added to processing queue")
	if err == nil {
		defer h.apis.Bot.DeleteMessage(req.ChatID, msg.MessageID)
	}

	err = <-req.ErrChan
	if err != nil {
		_, _ = h.apis.Bot.SendMessage(req.ChatID, "Unknown error while processing emote")

		slog.Error(
			"MediaHandler.CreateVideoFromEmote",
			slog.Int64("chatID", req.ChatID),
			slog.String("emoteID", req.EmoteID),
			slog.Any("err", err),
		)
	}
}

func (h *Handler) mediaWorker() {
	for req := range h.reqQueue {
		err := h.processEmote(req.ChatID, req.ReplyToMessageID, req.EmoteID)
		if err != nil {
			req.ErrChan <- err
		}
		close(req.ErrChan)
	}
}

func (h *Handler) validateUserInput(inp string) (emoteID string, err error) {
	errMsg := errors.Wrap(
		errors.New("invalid input"),
		"MediaHandler.validateUserInput",
	)

	trimmed := strings.TrimPrefix(inp, "https://")
	trimmed = strings.TrimPrefix(trimmed, "www.")
	if !strings.HasPrefix(trimmed, "7tv.app/emotes") {
		return "", errMsg
	}

	u, err := url.ParseRequestURI(inp)
	if err != nil {
		return "", errMsg
	}

	parts := strings.Split(strings.TrimLeft(u.Path, "/"), "/")
	if len(parts) != 2 {
		return "", errMsg
	}

	return parts[1], nil
}

func (h *Handler) processEmote(chatID int64, replyToMessageID int, emoteID string) error {
	const errMsg = "processEmote"

	var err error
	var webpFilePath, webmFilePath string

	defer func() {
		_ = os.RemoveAll(webpFilePath)
		_ = os.RemoveAll(webmFilePath)
	}()

	webpFilePath, err = h.apis.SevenTV.DownloadWebp(emoteID)
	if err != nil {
		return errors.Wrap(err, errMsg)
	}

	webmFilePath, err = h.services.Media.ConvertToTelegramVideo(webpFilePath)
	if err != nil {
		return errors.Wrap(err, errMsg)
	}

	attachment := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(webmFilePath))
	attachment.ReplyToMessageID = replyToMessageID

	err = h.apis.Bot.SendAttachment(attachment)
	if err != nil {
		return errors.Wrap(err, errMsg)
	}

	return nil
}
