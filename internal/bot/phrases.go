package bot

import "fmt"

// Тексты для пользователей. Все на русском; новые тексты добавляй сюда,
// а не в обработчики.

// Приём объявления: ответы автору до публикации.
const (
	// helpMessage — ответ на /start и /help: как подать объявление.
	helpMessage = "Пришлите объявление текстом или фото с подписью, можно альбомом из нескольких фото. " +
		"Бот опубликует его в канале с вашим именем и пришлёт кнопку «Не актуально»: нажмите её, когда объявление станет неактуальным"
	// helpSubscribersOnlyMessage добавляется к helpMessage, когда включена
	// проверка подписки (BOT_AUTH_ENABLED).
	helpSubscribersOnlyMessage = "Объявления принимаются только от подписчиков канала"
	// notSubscribedMessage — автор не состоит в канале; объявления принимаются только от подписчиков.
	notSubscribedMessage = "Объявления принимаются только от подписчиков канала. Подпишитесь и отправьте объявление заново"
	// authCheckFailedMessage — Telegram не ответил на проверку подписки.
	authCheckFailedMessage = "Не удалось проверить подписку на канал. Попробуйте позже"
	// noCaptionMessage — у фото или альбома нет подписи с текстом объявления.
	noCaptionMessage = "Фото без подписи не публикуются. Отправьте фото заново и добавьте подпись с текстом объявления"
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
	adPublishedMessage = "Объявление опубликовано.\nНажмите кнопку, когда оно станет неактуальным"
	// publishFailedMessage — объявление не удалось отправить в канал.
	publishFailedMessage = "Не удалось опубликовать объявление в канале. Попробуйте позже"
	// publishUncertainMessage — ответа Telegram на публикацию нет или он пустой,
	// и пост мог выйти в канал; повторная отправка дала бы дубль.
	publishUncertainMessage = "Не удалось дождаться ответа Telegram, объявление могло опубликоваться. Проверьте канал, прежде чем отправлять его снова"
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
