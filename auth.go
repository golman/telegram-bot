package main

import (
	"context"
	"log/slog"

	tgbotapi "github.com/mymmrac/telego"
)

// authByChannel проверяет, что автор сообщения состоит в канале.
// Ошибка возвращается, если Telegram не ответил на GetChatMember:
// в этом случае подписка неизвестна, а не отсутствует.
func (vbbot *VBBot) authByChannel(update tgbotapi.Update) (bool, error) {
	if !vbbot.cfg.authEnabled {
		return true, nil
	}
	cmc := tgbotapi.GetChatMemberParams{}
	cmc.UserID = update.Message.From.ID
	cmc.ChatID = tgbotapi.ChatID{ID: vbbot.channelId}
	cm, err := vbbot.tgbot.GetChatMember(context.Background(), &cmc)

	if err != nil {
		return false, err
	}
	slog.Debug("статус в канале", "user_id", cmc.UserID, "status", cm.MemberStatus())
	return cm.MemberIsMember(), nil
}
