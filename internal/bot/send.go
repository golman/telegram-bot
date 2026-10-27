package bot

import (
	"context"
	"errors"
	"log/slog"

	tgbotapi "github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"

	"github.com/golman/telegram-bot/internal/post"
)

// errEmptyMediaGroupResponse — Bot API ответил на sendMediaGroup без сообщений,
// и ID поста для кнопки «Не актуально» взять неоткуда.
var errEmptyMediaGroupResponse = errors.New("пустой ответ на sendMediaGroup")

func (vbbot *VBBot) sendMediaGroup(mm *MediaMessage) {
	if len(mm.fileid) == 0 {
		// В альбоме только видео или файлы.
		vbbot.sendTextMessage(UnsupportedMessage, mm.userid)
		return
	}
	if mm.caption == "" {
		vbbot.sayNoEmptyMessage(mm.userid)
		return
	}

	src := post.Source{
		Text:       mm.caption,
		Entities:   mm.entities,
		AuthorID:   mm.userid,
		AuthorName: mm.fullname,
		Media:      true,
	}
	if vbbot.rejectTooLong(src, mm.userid, mm.originalmessageid) {
		return
	}
	text, entities := post.Build(src, false)
	mgc := tgbotapi.SendMediaGroupParams{
		ChatID: tgbotapi.ChatID{ID: vbbot.channelId},
		Media:  make([]tgbotapi.InputMedia, 0),
	}

	for i := range mm.fileid {
		ph := mm.fileid[i]
		imp := tgbotapi.InputMediaPhoto{
			Type: "photo",
			Media: tgbotapi.InputFile{
				FileID: ph,
			},
		}
		if len(mgc.Media) == 0 {
			imp.Caption = text
			imp.CaptionEntities = entities
		}
		mgc.Media = append(mgc.Media, &imp)
	}

	sentMsg, err := vbbot.tgbot.SendMediaGroup(context.Background(), &mgc)
	if err == nil && len(sentMsg) == 0 {
		err = errEmptyMediaGroupResponse
	}
	if err != nil {
		vbbot.reportPublishError(err, mm.userid)
	} else {
		vbbot.confirmNewAd(mm.userid, mm.originalmessageid, sentMsg[0])
		if mm.skipped > 0 {
			vbbot.sendTextMessage(AlbumSkippedMessage, mm.userid)
		}
	}

}

// sendPhotoMessage публикует одиночное фото с подписью. Пост собирается
// той же post.FromMessage, что и при нажатии кнопки, поэтому
// публикация и пометка «Не актуально» берут текст одинаково.
func (vbbot *VBBot) sendPhotoMessage(update tgbotapi.Update) {
	src, ok := post.FromMessage(update.Message)
	if !ok {
		vbbot.sayNoEmptyMessage(update.Message.Chat.ID)
		return
	}

	// Получение фотографии с наибольшим размером
	photo := (update.Message.Photo)[len(update.Message.Photo)-1]

	if vbbot.rejectTooLong(src, update.Message.Chat.ID, update.Message.MessageID) {
		return
	}
	text, ents := post.Build(src, false)
	msg := tgbotapi.SendMediaGroupParams{
		ChatID: tgbotapi.ChatID{ID: vbbot.channelId},
		Media: []tgbotapi.InputMedia{
			&tgbotapi.InputMediaPhoto{
				Type: "photo",
				Media: tgbotapi.InputFile{
					FileID: photo.FileID,
				},
				Caption:         text,
				CaptionEntities: ents,
			},
		},
	}

	// Отправка сообщения с фотографией и подписью
	sentMsg, err := vbbot.tgbot.SendMediaGroup(context.Background(), &msg)
	if err == nil && len(sentMsg) == 0 {
		err = errEmptyMediaGroupResponse
	}
	if err != nil {
		vbbot.reportPublishError(err, update.Message.Chat.ID)
	} else {
		vbbot.confirmNewAd(update.Message.From.ID, update.Message.MessageID, sentMsg[0])
	}
}

// sendPlainMessage публикует текстовое объявление; пост собирается
// post.FromMessage, как и в sendPhotoMessage.
func (vbbot *VBBot) sendPlainMessage(update tgbotapi.Update) {
	src, ok := post.FromMessage(update.Message)
	if !ok {
		vbbot.sayNoEmptyMessage(update.Message.Chat.ID)
		return
	}
	if vbbot.rejectTooLong(src, update.Message.Chat.ID, update.Message.MessageID) {
		return
	}
	text, ents := post.Build(src, false)
	emsg := &tgbotapi.SendMessageParams{
		ChatID:   tgbotapi.ChatID{ID: vbbot.channelId},
		Text:     text,
		Entities: ents,
	}
	sentMsg, err := vbbot.tgbot.SendMessage(context.Background(), emsg)
	if err != nil {
		vbbot.reportPublishError(err, update.Message.Chat.ID)
	} else {
		vbbot.confirmNewAd(update.Message.From.ID, update.Message.MessageID, *sentMsg)
	}
}

// sayNoEmptyMessage отвечает автору, что у объявления нет текста или подписи.
func (vbbot *VBBot) sayNoEmptyMessage(chatId int64) {
	vbbot.sendTextMessage(EmptyMessage, chatId)
}

// rejectTooLong проверяет, уместится ли пост из src в лимит Telegram.
// Если нет, отвечает автору в чате chatID на его сообщение replyTo
// и возвращает true. Такой пост не отправляется.
func (vbbot *VBBot) rejectTooLong(src post.Source, chatID int64, replyTo int) bool {
	n, limit := telegoutil.UTF16TextLen(src.Text), post.TextLimit(src)
	if n <= limit {
		return false
	}
	slog.Info("объявление слишком длинное", "user_id", src.AuthorID, "length", n, "limit", limit)
	msg := tgbotapi.SendMessageParams{
		ChatID: tgbotapi.ChatID{ID: chatID},
		Text:   postTooLongText(n - limit),
		// Предупреждение уйдёт, даже если автор уже удалил исходное сообщение.
		ReplyParameters: &tgbotapi.ReplyParameters{MessageID: replyTo, AllowSendingWithoutReply: true},
	}
	if err := vbbot.sendMessage(&msg); err != nil {
		slog.Warn("не удалось отправить сообщение", "chat_id", chatID, "err", err)
	}
	return true
}

func (vbbot *VBBot) sendTextMessage(txtMsg string, chatId int64) {
	botMsg := tgbotapi.SendMessageParams{
		ChatID: tgbotapi.ChatID{ID: chatId},
		Text:   txtMsg,
	}
	if err := vbbot.sendMessage(&botMsg); err != nil {
		// Второе сообщение в тот же чат, скорее всего, тоже не дойдёт.
		slog.Warn("не удалось отправить сообщение", "chat_id", chatId, "err", err)
	}
}

func (vbbot *VBBot) sendMessage(msg *tgbotapi.SendMessageParams) error {
	_, err := vbbot.tgbot.SendMessage(context.Background(), msg)
	return err
}

// reportPublishError логирует ошибку публикации объявления и сообщает автору
// в чат chatID понятный текст. Сырой текст ошибки API автору не показывается.
func (vbbot *VBBot) reportPublishError(err error, chatID int64) {
	slog.Error("не удалось опубликовать объявление", "chat_id", chatID, "err", err)
	vbbot.sendTextMessage(PublishFailedMessage, chatID)
}
