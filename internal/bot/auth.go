package bot

import (
	"context"
	"log/slog"

	"github.com/mymmrac/telego"
)

// authByChannel проверяет, что автор сообщения состоит в канале.
// Ошибка возвращается, если Telegram не ответил на GetChatMember:
// в этом случае подписка неизвестна, а не отсутствует.
func (b *Bot) authByChannel(msg *telego.Message) (bool, error) {
	if !b.authEnabled {
		return true, nil
	}
	cmc := telego.GetChatMemberParams{}
	cmc.UserID = msg.From.ID
	cmc.ChatID = telego.ChatID{ID: b.channelID}
	cm, err := b.client.GetChatMember(context.Background(), &cmc)

	if err != nil {
		return false, err
	}
	slog.Debug("статус в канале", "user_id", cmc.UserID, "status", cm.MemberStatus())
	return cm.MemberIsMember(), nil
}
