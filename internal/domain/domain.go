package domain

type UserRequest struct {
	ChatID           int64
	ReplyToMessageID int
	EmoteIDs         []string
	ErrChan          chan error
}

type EmotePaths struct {
	Webp string
	Webm string
}

type EmoteLayer struct {
	WebmPath string
	Duration float64
}
