package main

import (
	"fmt"
	"log"

	tgbotapi "github.com/mymmrac/telego"
)

func (vbbot *VBBot) sendMediaGroupMessage(config *tgbotapi.SendMediaGroupParams) error {
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
	mgc := tgbotapi.SendMediaGroupParams{
		ChatID: tgbotapi.ChatID{ID: vbbot.channelId},
		Media:  make([]tgbotapi.InputMedia, 0),
	}

	for i := range mm.fileid {
		ph := mm.fileid[i]
		//imp := tgbotapi.NewInputMediaPhoto(tgbotapi.FileID(ph))
		imp := tgbotapi.InputMediaPhoto{
			Type: "photo",
			Media: tgbotapi.InputFile{
				FileID: ph,
			},
		}
		if len(mgc.Media) == 0 {
			imp.Caption = caption
			imp.ParseMode = tgbotapi.ModeMarkdownV2
		}
		mgc.Media = append(mgc.Media, &imp)
	}

	err := vbbot.sendMediaGroupMessage(&mgc)
	if err != nil {
		vbbot.handleError(err, mm.userid)
	}

}

func (vbbot *VBBot) sendPhotoMessage(update tgbotapi.Update) {
	if update.Message.Caption == "" {
		vbbot.sayNoEmptyMessage(update.Message.Chat.ID)
		return
	}

	// Получение фотографии с наибольшим размером
	photo := (update.Message.Photo)[len(update.Message.Photo)-1]

	// Изменение подписи
	caption := createCaption(update.Message.Caption,
		update.Message.From.FirstName+" "+update.Message.From.LastName,
		update.Message.From.ID)

	// Создание сообщения с фотографией и измененной подписью
	//msg := tgbotapi.NewPhoto(vbbot.channelId, tgbotapi.FileID(fileConfig.FileID))
	msg := tgbotapi.SendMediaGroupParams{
		ChatID: tgbotapi.ChatID{ID: vbbot.channelId},
		Media: []tgbotapi.InputMedia{
			&tgbotapi.InputMediaPhoto{
				Type: "photo",
				Media: tgbotapi.InputFile{
					FileID: photo.FileID,
				},
				Caption:   caption,
				ParseMode: tgbotapi.ModeMarkdownV2,
			},
		},
	}

	// Отправка сообщения с фотографией и подписью
	err := vbbot.sendMedia(&msg)
	if err != nil {
		vbbot.handleError(err, update.Message.Chat.ID)
	}
}

func (vbbot *VBBot) sendPlainMessage(update tgbotapi.Update) {
	if update.Message.Text == "" {
		vbbot.sayNoEmptyMessage(update.Message.Chat.ID)
		return
	}
	msg := tgbotapi.SendMessageParams{
		ChatID: tgbotapi.ChatID{ID: vbbot.channelId},
		Text:   update.Message.Text,
	}
	msg.Text = createCaption(msg.Text,
		update.Message.From.FirstName+" "+update.Message.From.LastName,
		update.Message.From.ID)
	msg.ParseMode = tgbotapi.ModeMarkdownV2
	err := vbbot.sendMessage(&msg)
	if err != nil {
		vbbot.handleError(err, update.Message.Chat.ID)
	}
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
	_, err := vbbot.tgbot.SendMessage(msg)
	return err
}

func (vbbot *VBBot) sendMedia(msg *tgbotapi.SendMediaGroupParams) error {
	_, err := vbbot.tgbot.SendMediaGroup(msg)
	return err
}

func (vbbot *VBBot) handleError(msgErr error, chatId int64) {
	log.Println(msgErr)
	errmsg := fmt.Sprintf("Не удалось отправить сообщение. %s", msgErr.Error())
	botMsg := tgbotapi.SendMessageParams{
		ChatID: tgbotapi.ChatID{ID: chatId},
		Text:   errmsg,
	}
	_, err := vbbot.tgbot.SendMessage(&botMsg)
	if err != nil {
		log.Println(err)
	}
}
