package main

import (
	"fmt"
	"log"

	tgbotapi "github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
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

	text, entities := telegoutil.MessageEntities(
		telegoutil.Entity(mm.caption),
		telegoutil.Entity("\n\n by: "),
		telegoutil.Entity(mm.fullname).TextMentionWithID(mm.userid),
	)
	entities = append(entities, mm.entities...)
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

	sentMsg, err := vbbot.tgbot.SendMediaGroup(&mgc)
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

	text, ents := telegoutil.MessageEntities(
		telegoutil.Entity(update.Message.Caption),
		telegoutil.Entity("\n\n by: "),
		telegoutil.Entity(update.Message.From.FirstName+" "+update.Message.From.LastName).TextMentionWithID(update.Message.From.ID),
	)
	ents = append(ents, update.Message.CaptionEntities...)
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
				Caption:         text,
				CaptionEntities: ents,
			},
		},
	}

	// Отправка сообщения с фотографией и подписью
	sentMsg, err := vbbot.tgbot.SendMediaGroup(&msg)
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
	emsg := telegoutil.MessageWithEntities(tgbotapi.ChatID{ID: vbbot.channelId},
		telegoutil.Entity(update.Message.Text).Bold(),
		telegoutil.Entity("\n\n by: "),
		telegoutil.Entity(update.Message.From.FirstName+" "+update.Message.From.LastName).TextMentionWithID(update.Message.From.ID),
	)
	emsg.Entities = append(emsg.Entities, update.Message.Entities...)
	sentMsg, err := vbbot.tgbot.SendMessage(emsg)
	if err != nil {
		vbbot.handleError(err, update.Message.Chat.ID)
	} else {
		vbbot.confirmNewAd(update.Message.From.ID, update.Message.MessageID, *sentMsg)
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
