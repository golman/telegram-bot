package bot

import (
	"context"
	"errors"
	"log/slog"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/golman/telegram-bot/internal/post"
)

// errEmptyMediaGroupResponse — Bot API ответил на sendMediaGroup без сообщений,
// и ID поста для кнопки «Не актуально» взять неоткуда.
var errEmptyMediaGroupResponse = errors.New("пустой ответ на sendMediaGroup")

// errEmptyPhotoResponse — Bot API ответил на sendPhoto без сообщения.
var errEmptyPhotoResponse = errors.New("пустой ответ на sendPhoto")

// errEmptyMessageResponse — Bot API ответил на sendMessage без сообщения.
var errEmptyMessageResponse = errors.New("пустой ответ на sendMessage")

func (b *Bot) sendMediaGroup(a *album) {
	if len(a.fileIDs) == 0 {
		// В альбоме только видео или файлы.
		b.sendTextMessage(unsupportedMessage, a.authorID)
		return
	}
	if a.caption == "" {
		b.sayNoCaption(a.authorID)
		return
	}

	src := post.Source{
		Text:       a.caption,
		Entities:   a.entities,
		AuthorID:   a.authorID,
		AuthorName: a.authorName,
		Media:      true,
	}
	if b.rejectTooLong(src, a.authorID, a.replyToID) {
		return
	}
	text, entities := post.Build(src, false)
	mgc := telego.SendMediaGroupParams{
		ChatID: telego.ChatID{ID: b.channelID},
		Media:  make([]telego.InputMedia, 0),
	}

	for i := range a.fileIDs {
		ph := a.fileIDs[i]
		imp := telego.InputMediaPhoto{
			Type: "photo",
			Media: telego.InputFile{
				FileID: ph,
			},
		}
		if len(mgc.Media) == 0 {
			imp.Caption = text
			imp.CaptionEntities = entities
		}
		mgc.Media = append(mgc.Media, &imp)
	}

	sentMsg, err := b.client.SendMediaGroup(context.Background(), &mgc)
	if err == nil && len(sentMsg) == 0 {
		err = errEmptyMediaGroupResponse
	}
	if err != nil {
		b.reportPublishError(err, a.authorID)
	} else {
		a.published = true
		b.confirmNewAd(a.authorID, a.replyToID, sentMsg[0])
		if a.skipped > 0 {
			b.sendTextMessage(albumSkippedMessage, a.authorID)
		}
	}

}

// sendPhotoMessage публикует одиночное фото с подписью. Пост собирается
// той же post.FromMessage, что и при нажатии кнопки, поэтому
// публикация и пометка «Не актуально» берут текст одинаково.
func (b *Bot) sendPhotoMessage(update telego.Update) {
	src, ok := post.FromMessage(update.Message)
	if !ok {
		b.sayNoCaption(update.Message.Chat.ID)
		return
	}

	// Получение фотографии с наибольшим размером
	photo := (update.Message.Photo)[len(update.Message.Photo)-1]

	if b.rejectTooLong(src, update.Message.Chat.ID, update.Message.MessageID) {
		return
	}
	text, ents := post.Build(src, false)
	msg := telego.SendPhotoParams{
		ChatID:          telego.ChatID{ID: b.channelID},
		Photo:           telego.InputFile{FileID: photo.FileID},
		Caption:         text,
		CaptionEntities: ents,
	}

	sent, err := b.client.SendPhoto(context.Background(), &msg)
	if err == nil && sent == nil {
		err = errEmptyPhotoResponse
	}
	if err != nil {
		b.reportPublishError(err, update.Message.Chat.ID)
		return
	}
	b.confirmNewAd(update.Message.From.ID, update.Message.MessageID, *sent)
}

// sendPlainMessage публикует текстовое объявление; пост собирается
// post.FromMessage, как и в sendPhotoMessage.
func (b *Bot) sendPlainMessage(update telego.Update) {
	src, ok := post.FromMessage(update.Message)
	if !ok {
		// handleUpdate не пропускает сообщения без текста и без автора,
		// так что сюда попасть не должно; отвечать автору здесь нечего.
		slog.Warn("сообщение без текста или автора, не опубликовано",
			"chat_id", update.Message.Chat.ID, "message_id", update.Message.MessageID)
		return
	}
	if b.rejectTooLong(src, update.Message.Chat.ID, update.Message.MessageID) {
		return
	}
	text, ents := post.Build(src, false)
	emsg := &telego.SendMessageParams{
		ChatID:   telego.ChatID{ID: b.channelID},
		Text:     text,
		Entities: ents,
	}
	sentMsg, err := b.client.SendMessage(context.Background(), emsg)
	if err == nil && sentMsg == nil {
		err = errEmptyMessageResponse
	}
	if err != nil {
		b.reportPublishError(err, update.Message.Chat.ID)
	} else {
		b.confirmNewAd(update.Message.From.ID, update.Message.MessageID, *sentMsg)
	}
}

// sayNoCaption отвечает автору, что у фото или альбома нет подписи.
func (b *Bot) sayNoCaption(chatID int64) {
	b.sendTextMessage(noCaptionMessage, chatID)
}

// rejectTooLong проверяет, уместится ли пост из src в лимит Telegram.
// Если нет, отвечает автору в чате chatID на его сообщение replyTo
// и возвращает true. Такой пост не отправляется.
func (b *Bot) rejectTooLong(src post.Source, chatID int64, replyTo int) bool {
	n, limit := tu.UTF16TextLen(src.Text), post.TextLimit(src)
	if n <= limit {
		return false
	}
	slog.Info("объявление слишком длинное", "user_id", src.AuthorID, "length", n, "limit", limit)
	msg := telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text:   postTooLongText(n - limit),
		// Предупреждение уйдёт, даже если автор уже удалил исходное сообщение.
		ReplyParameters: &telego.ReplyParameters{MessageID: replyTo, AllowSendingWithoutReply: true},
	}
	if err := b.sendMessage(&msg); err != nil {
		slog.Warn("не удалось отправить сообщение", "chat_id", chatID, "err", err)
	}
	return true
}

func (b *Bot) sendTextMessage(txtMsg string, chatID int64) {
	botMsg := telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text:   txtMsg,
	}
	if err := b.sendMessage(&botMsg); err != nil {
		// Второе сообщение в тот же чат, скорее всего, тоже не дойдёт.
		slog.Warn("не удалось отправить сообщение", "chat_id", chatID, "err", err)
	}
}

func (b *Bot) sendMessage(msg *telego.SendMessageParams) error {
	_, err := b.client.SendMessage(context.Background(), msg)
	return err
}

// reportPublishError логирует ошибку публикации объявления и сообщает автору
// в чат chatID понятный текст. Сырой текст ошибки API автору не показывается.
func (b *Bot) reportPublishError(err error, chatID int64) {
	slog.Error("не удалось опубликовать объявление", "chat_id", chatID, "err", err)
	b.sendTextMessage(publishFailedMessage, chatID)
}
