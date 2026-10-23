package main

const (
	MediaGroupDelayMessage = "Сообщение будет отправлено в канал через 10 секунд"
	NotSubscribedMessage   = "Вы не подписаны на канал"
	// AuthCheckFailedMessage отправляется, если Telegram не ответил на проверку подписки.
	AuthCheckFailedMessage = "Не удалось проверить подписку на канал. Попробуйте позже"
	FiltersFailedMessage   = "Сообщение не прошло фильтры и не будет отправлено в канал"
	DocumentsNotAllowed    = "Документы не принимаются"

	// DeprecatedPrefix ставится перед зачёркнутым текстом неактуального объявления.
	DeprecatedPrefix = "Не актуально.\n\n"
	// AuthorSuffix отделяет текст объявления от упоминания автора.
	AuthorSuffix = "\n\n by: "

	// Кнопки под подтверждением публикации.
	DeprecateButton = "Не актуально"
	RestoreButton   = "Восстановить"

	// Ответы на нажатие кнопки.
	AdDeprecatedMessage           = "Объявление помечено как неактуальное"
	AdRestoredMessage             = "Объявление восстановлено"
	CallbackInvalidMessage        = "Кнопка устарела или повреждена"
	OriginalMessageMissingMessage = "Исходное сообщение не найдено, изменить объявление нельзя"
	ChannelEditFailedMessage      = "Не удалось изменить объявление в канале"
)

func (vbbot *VBBot) sayNoEmptyMessage(chatId int64) {
	vbbot.sendTextMessage("Сообщение не может быть пустым", chatId)
}
