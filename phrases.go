package main

import "fmt"

const (
	MediaGroupDelayMessage = "Сообщение будет отправлено в канал через 10 секунд"
	// AlbumItemLateMessage отправляется, если фото альбома пришли после его публикации.
	AlbumItemLateMessage = "Часть фото из альбома пришла после публикации и не попала в объявление. Если они нужны, отправьте объявление заново одним альбомом"
	NotSubscribedMessage = "Вы не подписаны на канал"
	// AuthCheckFailedMessage отправляется, если Telegram не ответил на проверку подписки.
	AuthCheckFailedMessage = "Не удалось проверить подписку на канал. Попробуйте позже"
	FiltersFailedMessage   = "Сообщение не прошло фильтры и не будет отправлено в канал"
	// PostTooLongMessage — шаблон: на сколько сократить текст и слово «символ»
	// в нужной форме (см. postTooLongText).
	PostTooLongMessage  = "Объявление слишком длинное: сократите текст примерно на %d %s и отправьте заново"
	DocumentsNotAllowed = "Документы не принимаются"

	// DeprecatedPrefix ставится перед зачёркнутым текстом неактуального объявления.
	DeprecatedPrefix = "Не актуально.\n\n"
	// AuthorSuffix отделяет текст объявления от упоминания автора.
	AuthorSuffix = "\n\n by: "

	// Кнопки под подтверждением публикации.
	DeprecateButton = "Не актуально"
	RestoreButton   = "Восстановить"

	// AdPublishedMessage — подтверждение публикации, к нему прикреплена кнопка.
	AdPublishedMessage = "Объявление опубликовано.\nНажми на кнопку чтобы отметить как неактуальное."
	// PublishFailedMessage — объявление не удалось отправить в канал.
	PublishFailedMessage = "Не удалось опубликовать объявление в канале. Попробуйте позже"
	// ConfirmFailedMessage — пост в канале есть, но подтверждение с кнопкой не ушло.
	ConfirmFailedMessage = "Объявление опубликовано, но кнопку «Не актуально» отправить не удалось"

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

// postTooLongText возвращает PostTooLongMessage для превышения на over символов,
// со словом «символ» в форме, согласованной с числом.
func postTooLongText(over int) string {
	word := "символов"
	if n := over % 100; n < 11 || n > 14 {
		switch over % 10 {
		case 1:
			word = "символ"
		case 2, 3, 4:
			word = "символа"
		}
	}
	return fmt.Sprintf(PostTooLongMessage, over, word)
}
