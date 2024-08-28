package main

import (
	"log"

	tgbotapi "github.com/mymmrac/telego"
)

func (vbbot *VBBot) authByChannel(update tgbotapi.Update) bool {
	if !vbbot.cfg.authEnabled {
		return true
	}
	cmc := tgbotapi.GetChatMemberParams{}
	cmc.UserID = update.Message.From.ID
	cmc.ChatID = tgbotapi.ChatID{ID: vbbot.channelId}
	cm, err := vbbot.tgbot.GetChatMember(&cmc)

	if err != nil {
		log.Fatalln(err)
		return false
	}
	log.Println(cm)
	return cm.MemberIsMember()
}
