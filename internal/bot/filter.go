package bot

import tgbotapi "github.com/mymmrac/telego"

// runFilter возвращает true, если объявление нужно отклонить по содержимому.
// Пока фильтров нет: тип сообщения (только текст и фото) проверяет handleUpdate.
func (vbbot *VBBot) runFilter(update tgbotapi.Update) bool {
	return false
}
