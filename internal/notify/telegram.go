package notify

import (
	"context"
	"fmt"

	"github.com/mymmrac/telego"
)

// Telegram delivers balance updates to configured chats.
type Telegram struct {
	bot *telego.Bot
}

// NewTelegram builds telego bot instance.
func NewTelegram(token string) (*Telegram, error) {
	bot, err := telego.NewBot(token)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %v", err)
	}
	return &Telegram{bot: bot}, nil
}

// Notify sends message to every chat id.
func (t *Telegram) Notify(ctx context.Context, chatIDs []int64, message string) error {
	for _, chatID := range chatIDs {
		params := &telego.SendMessageParams{
			ChatID: telego.ChatID{ID: chatID},
			Text:   message,
		}
		if _, err := t.bot.SendMessage(ctx, params); err != nil {
			return fmt.Errorf("send message to %d: %v", chatID, err)
		}
	}
	return nil
}
