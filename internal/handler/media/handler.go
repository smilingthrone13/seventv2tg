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
	"golang.org/x/sync/errgroup"

	"seventv2tg/internal/config"
	"seventv2tg/internal/domain"
	"seventv2tg/internal/infrastructure/webapi"
	"seventv2tg/internal/service"
)

const workerCount = 3
const emoteIdLength = 26
const maxOverlayedEmotes = 3

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
	if _, ok := h.activityCache.Get(strconv.FormatInt(message.Chat.ID, 10)); ok {
		_, _ = h.apis.TgBot.SendMessage(message.Chat.ID, "You have another emote being processed, please wait")
		return
	}

	userInput := strings.Fields(message.Text)
	userInput = userInput[:min(len(userInput), maxOverlayedEmotes)]

	var emoteIDs []string

	for i := range userInput {
		emoteID, err := h.validateUserInput(userInput[i])
		if err != nil {
			_, _ = h.apis.TgBot.SendMessage(message.Chat.ID, "Invalid emote URL")
			return
		}

		emoteIDs = append(emoteIDs, emoteID)
	}

	h.activityCache.Set(strconv.FormatInt(message.Chat.ID, 10), struct{}{}, cache.NoExpiration)
	defer h.activityCache.Delete(strconv.FormatInt(message.Chat.ID, 10))

	req := domain.UserRequest{
		ChatID:           message.Chat.ID,
		ReplyToMessageID: message.MessageID,
		EmoteIDs:         emoteIDs,
		ErrChan:          make(chan error),
	}
	h.reqQueue <- req

	msg, err := h.apis.TgBot.SendMessage(req.ChatID, "Emote added to processing queue")
	if err == nil {
		defer h.apis.TgBot.DeleteMessage(req.ChatID, msg.MessageID)
	}

	err = <-req.ErrChan
	if err != nil {
		_, _ = h.apis.TgBot.SendMessage(req.ChatID, "Unknown error while processing emote")

		slog.Error(
			"MediaHandler.CreateVideoFromEmote",
			slog.Int64("chatID", req.ChatID),
			slog.Any("emoteIDs", req.EmoteIDs),
			slog.Any("err", err.Error()),
		)
	}
}

func (h *Handler) mediaWorker() {
	var err error

	for req := range h.reqQueue {
		if len(req.EmoteIDs) > 1 {
			err = h.processOverlayedEmote(req.ChatID, req.ReplyToMessageID, req.EmoteIDs)
		} else {
			err = h.processSingleEmote(req.ChatID, req.ReplyToMessageID, req.EmoteIDs[0])
		}
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

	trimmed := strings.TrimSpace(inp)
	trimmed = strings.TrimPrefix(trimmed, "https://")
	trimmed = strings.TrimPrefix(trimmed, "www.")

	if !strings.HasPrefix(trimmed, "7tv.app/emotes") {
		return "", errMsg
	}

	u, err := url.ParseRequestURI(inp)
	if err != nil {
		return "", errMsg
	}

	parts := strings.Split(strings.TrimLeft(u.Path, "/"), "/")
	if len(parts) != 2 || len(parts[1]) != emoteIdLength {
		return "", errMsg
	}

	return parts[1], nil
}

func (h *Handler) processSingleEmote(chatID int64, replyToMessageID int, emoteID string) error {
	const errMsg = "processSingleEmote"

	var err error
	var paths domain.EmotePaths

	defer func() {
		_ = os.RemoveAll(paths.Webp)
		_ = os.RemoveAll(paths.Webm)
	}()

	paths.Webp, err = h.apis.SevenTV.DownloadWebp(emoteID)
	if err != nil {
		return errors.Wrap(err, errMsg)
	}

	paths.Webm, err = h.services.Media.ConvertToVideo(paths.Webp)
	if err != nil {
		return errors.Wrap(err, errMsg)
	}

	attachment := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(paths.Webm))
	attachment.ReplyToMessageID = replyToMessageID

	err = h.apis.TgBot.SendAttachment(attachment)
	if err != nil {
		return errors.Wrap(err, errMsg)
	}

	return nil
}

func (h *Handler) processOverlayedEmote(chatID int64, replyToMessageID int, emoteIDs []string) error {
	const errMsg = "processOverlayedEmote"

	var err error
	var resFilePath string

	webpPaths := make([]string, len(emoteIDs))

	defer func() {
		_ = os.RemoveAll(resFilePath)
		for i := range webpPaths {
			_ = os.RemoveAll(webpPaths[i])
		}
	}()

	eg := errgroup.Group{}
	for i := range emoteIDs {
		eg.Go(func() error {
			webpPaths[i], err = h.apis.SevenTV.DownloadWebp(emoteIDs[i])

			return err
		})
	}

	if err = eg.Wait(); err != nil {
		return errors.Wrap(err, errMsg)
	}

	resFilePath, err = h.services.Media.OverlayVideos(webpPaths)
	if err != nil {
		return errors.Wrap(err, errMsg)
	}

	attachment := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(resFilePath))
	attachment.ReplyToMessageID = replyToMessageID

	err = h.apis.TgBot.SendAttachment(attachment)
	if err != nil {
		return errors.Wrap(err, errMsg)
	}

	return nil
}
