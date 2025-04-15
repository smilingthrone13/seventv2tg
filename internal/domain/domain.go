package domain

type UserRequest struct {
	ChatID           int64
	ReplyToMessageID int
	EmoteID          string
	ErrChan          chan error
}
