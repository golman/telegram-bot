package main

import "fmt"

// Тексты для пользователей. Все на русском; новые тексты добавляй сюда,
// а не в обработчики.

// Приём объявления: ответы автору до публикации.
const (
	// NotSubscribedMessage — автор не состоит в канале.
	NotSubscribedMessage = "Вы не подписаны на канал"
	// AuthCheckFailedMessage — Telegram не ответил на проверку подписки.
	AuthCheckFailedMessage = "Не удалось проверить подписку на канал. Попробуйте позже"
	// FiltersFailedMessage — объявление отклонено фильтрами.
	FiltersFailedMessage = "Сообщение не прошло фильтры и не будет отправлено в канал"
	// EmptyMessage — у объявления нет текста или подписи.
	EmptyMessage = "Сообщение не может быть пустым"
	// UnsupportedMessage — сообщение не текст и не фото.
	UnsupportedMessage = "Такие сообщения не принимаются. Отправьте объявление текстом или фото с подписью"
	// PostTooLongMessage — шаблон: на сколько сократить текст и слово «символ»
	// в нужной форме (см. postTooLongText).
	PostTooLongMessage = "Объявление слишком длинное: сократите текст примерно на %d %s и отправьте заново"
)

// Альбомы.
const (
	// MediaGroupDelayMessage — альбом принят и будет опубликован после всех фото.
	MediaGroupDelayMessage = "Альбом будет опубликован, как только загрузятся все фото"
	// AlbumItemLateMessage — фото альбома пришли после его публикации.
	AlbumItemLateMessage = "Часть фото из альбома пришла после публикации и не попала в объявление. Если они нужны, отправьте объявление заново одним альбомом"
	// AlbumSkippedMessage — в опубликованном альбоме были видео или файлы, они пропущены.
	AlbumSkippedMessage = "Видео и файлы из альбома не опубликованы: принимаются только фото"
)

// Публикация.
const (
	// AdPublishedMessage — подтверждение публикации, к нему прикреплена кнопка.
	AdPublishedMessage = "Объявление опубликовано.\nНажми на кнопку чтобы отметить как неактуальное."
	// PublishFailedMessage — объявление не удалось отправить в канал.
	PublishFailedMessage = "Не удалось опубликовать объявление в канале. Попробуйте позже"
	// ConfirmFailedMessage — пост в канале есть, но подтверждение с кнопкой не ушло.
	ConfirmFailedMessage = "Объявление опубликовано, но кнопку «Не актуально» отправить не удалось"
)

// Кнопки под подтверждением и ответы на их нажатие.
const (
	// DeprecateButton помечает объявление неактуальным.
	DeprecateButton = "Не актуально"
	// RestoreButton снимает пометку.
	RestoreButton = "Восстановить"

	// AdDeprecatedMessage — объявление помечено неактуальным.
	AdDeprecatedMessage = "Объявление помечено как неактуальное"
	// AdRestoredMessage — пометка снята.
	AdRestoredMessage = "Объявление восстановлено"
	// CallbackInvalidMessage — callback data не разобрались или ведут не в канал бота.
	CallbackInvalidMessage = "Кнопка устарела или повреждена"
	// OriginalMessageMissingMessage — исходное сообщение автора удалено или недоступно.
	OriginalMessageMissingMessage = "Исходное сообщение не найдено, изменить объявление нельзя"
	// ChannelEditFailedMessage — Telegram не дал изменить пост в канале.
	ChannelEditFailedMessage = "Не удалось изменить объявление в канале"
)

// Вид поста в канале (собирает buildPost).
const (
	// DeprecatedPrefix ставится перед зачёркнутым текстом неактуального объявления.
	DeprecatedPrefix = "Не актуально.\n\n"
	// AuthorSuffix отделяет текст объявления от упоминания автора.
	AuthorSuffix = "\n\n by: "
)

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
