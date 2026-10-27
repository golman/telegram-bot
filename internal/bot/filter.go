package bot

import "github.com/mymmrac/telego"

// runFilter возвращает true, если объявление нужно отклонить по содержимому.
// Пока фильтров нет: тип сообщения (только текст и фото) проверяет handleUpdate.
func (b *Bot) runFilter(update telego.Update) bool {
	return false
}
