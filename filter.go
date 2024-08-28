package main

import tgbotapi "github.com/mymmrac/telego"

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
