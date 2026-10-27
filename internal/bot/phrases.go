package bot

import "fmt"

// Тексты для пользователей. Все на русском; новые тексты добавляй сюда,
// а не в обработчики.

// Приём объявления: ответы автору до публикации.
const (
	// notSubscribedMessage — автор не состоит в канале.
	notSubscribedMessage = "Вы не подписаны на канал"
	// authCheckFailedMessage — Telegram не ответил на проверку подписки.
	authCheckFailedMessage = "Не удалось проверить подписку на канал. Попробуйте позже"
	// filtersFailedMessage — объявление отклонено фильтрами.
	filtersFailedMessage = "Сообщение не прошло фильтры и не будет отправлено в канал"
	// emptyMessage — у объявления нет текста или подписи.
	emptyMessage = "Сообщение не может быть пустым"
	// unsupportedMessage — сообщение не текст и не фото.
	unsupportedMessage = "Такие сообщения не принимаются. Отправьте объявление текстом или фото с подписью"
	// postTooLongMessage — шаблон: на сколько сократить текст и слово «символ»
	// в нужной форме (см. postTooLongText).
	postTooLongMessage = "Объявление слишком длинное: сократите текст примерно на %d %s и отправьте заново"
)

// Альбомы.
const (
	// mediaGroupDelayMessage — альбом принят и будет опубликован после всех фото.
	mediaGroupDelayMessage = "Альбом будет опубликован, как только загрузятся все фото"
	// albumItemLateMessage — фото альбома пришли после его публикации.
	albumItemLateMessage = "Часть фото из альбома пришла после публикации и не попала в объявление. Если они нужны, отправьте объявление заново одним альбомом"
	// albumSkippedMessage — в опубликованном альбоме были видео или файлы, они пропущены.
	albumSkippedMessage = "Видео и файлы из альбома не опубликованы: принимаются только фото"
)

// Публикация.
const (
	// adPublishedMessage — подтверждение публикации, к нему прикреплена кнопка.
	adPublishedMessage = "Объявление опубликовано.\nНажми на кнопку чтобы отметить как неактуальное."
	// publishFailedMessage — объявление не удалось отправить в канал.
	publishFailedMessage = "Не удалось опубликовать объявление в канале. Попробуйте позже"
	// confirmFailedMessage — пост в канале есть, но подтверждение с кнопкой не ушло.
	confirmFailedMessage = "Объявление опубликовано, но кнопку «Не актуально» отправить не удалось"
)

// Кнопки под подтверждением и ответы на их нажатие.
const (
	// deprecateButton помечает объявление неактуальным.
	deprecateButton = "Не актуально"
	// restoreButton снимает пометку.
	restoreButton = "Восстановить"

	// adDeprecatedMessage — объявление помечено неактуальным.
	adDeprecatedMessage = "Объявление помечено как неактуальное"
	// adRestoredMessage — пометка снята.
	adRestoredMessage = "Объявление восстановлено"
	// callbackInvalidMessage — callback data не разобрались или ведут не в канал бота.
	callbackInvalidMessage = "Кнопка устарела или повреждена"
	// originalMessageMissingMessage — исходное сообщение автора удалено или недоступно.
	originalMessageMissingMessage = "Исходное сообщение не найдено, изменить объявление нельзя"
	// channelEditFailedMessage — Telegram не дал изменить пост в канале.
	channelEditFailedMessage = "Не удалось изменить объявление в канале"
)

// postTooLongText возвращает postTooLongMessage для превышения на over символов,
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
	return fmt.Sprintf(postTooLongMessage, over, word)
}
