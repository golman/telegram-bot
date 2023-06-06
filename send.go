package main

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
)

func (vbbot *VBBot) sendTextMessage(txtMsg string, chatId int64) {
	escaped := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, txtMsg)
	botMsg := tgbotapi.NewMessage(chatId, escaped)
	vbbot.sendMessage(botMsg)
}

func (vbbot *VBBot) sendMessage(msg tgbotapi.Chattable) {
	_, err := vbbot.tgbot.Send(msg)
	if err != nil {
		log.Println(err)
	}
}

func (vbbot *VBBot) sendMediaGroupMessage(config tgbotapi.MediaGroupConfig) {
	_, err := vbbot.tgbot.SendMediaGroup(config)
	if err != nil {
		log.Println(err)
	}
}

func (vbbot *VBBot) sendMediaGroup(mm *MediaMessage) {
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

	vbbot.sendMediaGroupMessage(mgc)

}

func (vbbot *VBBot) sendPhotoMessage(update tgbotapi.Update) {
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
	vbbot.sendMessage(msg)
}
