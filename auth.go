package main

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
)

func (vbbot *VBBot) authByChannel(update tgbotapi.Update) bool {
	cmc := tgbotapi.GetChatMemberConfig{}
	cmc.UserID = update.Message.From.ID
	cmc.ChatID = vbbot.channelId
	cm, err := vbbot.tgbot.GetChatMember(cmc)

	if err != nil {
		log.Fatalln(err)
		return false
	}
	log.Println(cm)
	return cm.IsMember || cm.IsCreator() || cm.IsAdministrator()
}
