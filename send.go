package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	tgbotapi "github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
)

// errEmptyMediaGroupResponse — Bot API ответил на sendMediaGroup без сообщений,
// и ID поста для кнопки «Не актуально» взять неоткуда.
var errEmptyMediaGroupResponse = errors.New("пустой ответ на sendMediaGroup")

func (vbbot *VBBot) sendMediaGroup(mm *MediaMessage) {
	if mm.caption == "" {
		vbbot.sayNoEmptyMessage(mm.userid)
		return
	}

	src := postSource{
		text:       mm.caption,
		entities:   mm.entities,
		authorID:   mm.userid,
		authorName: mm.fullname,
		media:      true,
	}
	if vbbot.rejectTooLong(src, mm.userid, mm.originalmessageid) {
		return
	}
	text, entities := buildPost(src, false)
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
		vbbot.handleError(err, mm.userid)
	} else {
		vbbot.confirmNewAd(mm.userid, mm.originalmessageid, sentMsg[0])
	}

}

func (vbbot *VBBot) sendPhotoMessage(update tgbotapi.Update) {
	if update.Message.Caption == "" {
		vbbot.sayNoEmptyMessage(update.Message.Chat.ID)
		return
	}

	// Получение фотографии с наибольшим размером
	photo := (update.Message.Photo)[len(update.Message.Photo)-1]

	src := postSource{
		text:       update.Message.Caption,
		entities:   update.Message.CaptionEntities,
		authorID:   update.Message.From.ID,
		authorName: displayName(*update.Message.From),
		media:      true,
	}
	if vbbot.rejectTooLong(src, update.Message.Chat.ID, update.Message.MessageID) {
		return
	}
	text, ents := buildPost(src, false)
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
		vbbot.handleError(err, update.Message.Chat.ID)
	} else {
		vbbot.confirmNewAd(update.Message.From.ID, update.Message.MessageID, sentMsg[0])
	}
}

func (vbbot *VBBot) sendPlainMessage(update tgbotapi.Update) {
	if update.Message.Text == "" {
		vbbot.sayNoEmptyMessage(update.Message.Chat.ID)
		return
	}
	src := postSource{
		text:       update.Message.Text,
		entities:   update.Message.Entities,
		authorID:   update.Message.From.ID,
		authorName: displayName(*update.Message.From),
	}
	if vbbot.rejectTooLong(src, update.Message.Chat.ID, update.Message.MessageID) {
		return
	}
	text, ents := buildPost(src, false)
	emsg := &tgbotapi.SendMessageParams{
		ChatID:   tgbotapi.ChatID{ID: vbbot.channelId},
		Text:     text,
		Entities: ents,
	}
	sentMsg, err := vbbot.tgbot.SendMessage(context.Background(), emsg)
	if err != nil {
		vbbot.handleError(err, update.Message.Chat.ID)
	} else {
		vbbot.confirmNewAd(update.Message.From.ID, update.Message.MessageID, *sentMsg)
	}
}

// rejectTooLong проверяет, уместится ли пост из src в лимит Telegram.
// Если нет, отвечает автору в чате chatID на его сообщение replyTo
// и возвращает true. Такой пост не отправляется.
func (vbbot *VBBot) rejectTooLong(src postSource, chatID int64, replyTo int) bool {
	n, limit := telegoutil.UTF16TextLen(src.text), postTextLimit(src)
	if n <= limit {
		return false
	}
	log.Printf("объявление пользователя %d слишком длинное: %d при максимуме %d", src.authorID, n, limit)
	msg := tgbotapi.SendMessageParams{
		ChatID: tgbotapi.ChatID{ID: chatID},
		Text:   postTooLongText(n - limit),
		// Предупреждение уйдёт, даже если автор уже удалил исходное сообщение.
		ReplyParameters: &tgbotapi.ReplyParameters{MessageID: replyTo, AllowSendingWithoutReply: true},
	}
	if err := vbbot.sendMessage(&msg); err != nil {
		vbbot.handleError(err, chatID)
	}
	return true
}

func (vbbot *VBBot) sendTextMessage(txtMsg string, chatId int64) {
	botMsg := tgbotapi.SendMessageParams{
		ChatID: tgbotapi.ChatID{ID: chatId},
		Text:   txtMsg,
	}
	err := vbbot.sendMessage(&botMsg)
	if err != nil {
		vbbot.handleError(err, chatId)
	}
}

func (vbbot *VBBot) sendMessage(msg *tgbotapi.SendMessageParams) error {
	_, err := vbbot.tgbot.SendMessage(context.Background(), msg)
	return err
}

func (vbbot *VBBot) handleError(msgErr error, chatId int64) {
	log.Println(msgErr)
	errmsg := fmt.Sprintf("Не удалось отправить сообщение. %s", msgErr.Error())
	botMsg := tgbotapi.SendMessageParams{
		ChatID: tgbotapi.ChatID{ID: chatId},
		Text:   errmsg,
	}
	_, err := vbbot.tgbot.SendMessage(context.Background(), &botMsg)
	if err != nil {
		log.Println(err)
	}
}
