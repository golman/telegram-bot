package main

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

func (vbbot *VBBot) checkIfContainsDocument(update tgbotapi.Update) bool {
	f := update.Message.Document != nil
	if f {
		vbbot.sendTextMessage(DocumentsNotAllowed, update.Message.Chat.ID)
	}
	return f
}

func (vbbot *VBBot) runFilter(update tgbotapi.Update) bool {
	return vbbot.checkIfContainsDocument(update)
}
