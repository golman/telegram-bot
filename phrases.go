package main

const (
	MediaGroupDelayMessage = "Сообщение будет отправлено в канал через 10 секунд"
	NotSubscribedMessage   = "Вы не подписаны на канал"
	FiltersFailedMessage   = "Сообщение не прошло фильтры и не будет отправлено в канал"
	DocumentsNotAllowed    = "Документы не принимаются"
)

func (vbbot *VBBot) sayNoEmptyMessage(chatId int64) {
	vbbot.sendTextMessage("Сообщение не может быть пустым", chatId)
}
