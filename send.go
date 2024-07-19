package main

import (
	"fmt"
	"log"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func (vbbot *VBBot) sendMediaGroupMessage(config tgbotapi.MediaGroupConfig) error {
	_, err := vbbot.tgbot.SendMediaGroup(config)
	return err
}

func (vbbot *VBBot) sendMediaGroup(mm *MediaMessage) {
	if mm.caption == "" {
		vbbot.sayNoEmptyMessage(mm.userid)
		return
	}
	// Изменение подписи
	caption := mm.createCaption()
	mgc := tgbotapi.MediaGroupConfig{
		ChatID: vbbot.channelId,
		Media:  make([]interface{}, 0),
	}

	for i := range mm.fileid {
		ph := mm.fileid[i]
		imp := tgbotapi.NewInputMediaPhoto(tgbotapi.FileID(ph))
		if len(mgc.Media) == 0 {
			imp.Caption = caption
			imp.ParseMode = tgbotapi.ModeMarkdownV2
		}
		mgc.Media = append(mgc.Media, imp)
	}

	sentMsg, err := vbbot.tgbot.SendMediaGroup(mgc)
	if err != nil {
		vbbot.handleError(err, mm.userid)
	} else {
		vbbot.confirmNewInzerat(mm.userid, mm.originalmessageid, sentMsg[0])
	}

}

func (vbbot *VBBot) sendPhotoMessage(update tgbotapi.Update) {
	if update.Message.Caption == "" {
		vbbot.sayNoEmptyMessage(update.Message.Chat.ID)
		return
	}

	// Получение фотографии с наибольшим размером
	photo := (update.Message.Photo)[len(update.Message.Photo)-1]

	// Запрос файла фотографии
	fileConfig := tgbotapi.FileConfig{
		FileID: photo.FileID,
	}
	// Изменение подписи
	caption := createCaption(update.Message.Caption,
		update.Message.From.FirstName+" "+update.Message.From.LastName,
		update.Message.From.ID)

	// Создание сообщения с фотографией и измененной подписью
	msg := tgbotapi.NewPhoto(vbbot.channelId, tgbotapi.FileID(fileConfig.FileID))
	msg.Caption = caption
	msg.ParseMode = tgbotapi.ModeMarkdownV2

	// Отправка сообщения с фотографией и подписью
	// err := vbbot.sendMessage(msg)
	sentMsg, err := vbbot.tgbot.Send(msg)
	if err != nil {
		vbbot.handleError(err, update.Message.Chat.ID)
	} else {
		vbbot.confirmNewInzerat(update.Message.From.ID, update.Message.MessageID, sentMsg)
	}
}

func (vbbot *VBBot) sendPlainMessage(update tgbotapi.Update) {
	if update.Message.Text == "" {
		vbbot.sayNoEmptyMessage(update.Message.Chat.ID)
		return
	}
	msg := tgbotapi.NewMessage(vbbot.channelId, update.Message.Text)
	msg.Text = createCaption(msg.Text,
		update.Message.From.FirstName+" "+update.Message.From.LastName,
		update.Message.From.ID)
	msg.ParseMode = tgbotapi.ModeMarkdownV2

	// Send text message
	sentMsg, err := vbbot.tgbot.Send(msg)
	if err != nil {
		vbbot.handleError(err, update.Message.Chat.ID)
		return
	} else {
		vbbot.confirmNewInzerat(update.Message.From.ID, update.Message.MessageID, sentMsg)
	}
}

func (vbbot *VBBot) confirmNewInzerat(chatId int64, replyToId int, sentMsg tgbotapi.Message) {
	buttonMsg := tgbotapi.NewMessage(chatId, "Объявление опубликовано.\nНажми на кнопку чтобы отметить проданным.")
	buttonMsg.ReplyToMessageID = replyToId
	buttonMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Кнопка!", "delete_"+strconv.Itoa(int(sentMsg.Chat.ID))+"_"+strconv.Itoa(sentMsg.MessageID)),
		),
	)
	if err := vbbot.sendMessage(buttonMsg); err != nil {
		vbbot.handleError(err, chatId)
	}
}

func (vbbot *VBBot) sendTextMessage(txtMsg string, chatId int64) {
	escaped := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, txtMsg)
	botMsg := tgbotapi.NewMessage(chatId, escaped)
	err := vbbot.sendMessage(botMsg)
	if err != nil {
		vbbot.handleError(err, chatId)
	}
}

func (vbbot *VBBot) sendMessage(msg tgbotapi.Chattable) error {
	_, err := vbbot.tgbot.Send(msg)
	return err
}

func (vbbot *VBBot) handleError(msgErr error, chatId int64) {
	log.Println(msgErr)
	errmsg := fmt.Sprintf("Не удалось отправить сообщение. %s", msgErr.Error())
	escaped := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, errmsg)
	botMsg := tgbotapi.NewMessage(chatId, escaped)
	_, err := vbbot.tgbot.Send(botMsg)
	if err != nil {
		log.Println(err)
	}
}
