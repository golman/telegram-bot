package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"

	"github.com/golman/telegram-bot/internal/post"
)

// callbackAction — действие inline-кнопки под подтверждением публикации.
type callbackAction string

const (
	actionDelete  callbackAction = "delete"
	actionRestore callbackAction = "restore"
)

// callbackData — разобранные данные кнопки: действие и пост в канале.
// Формат строки: "<действие>_<chatID>_<messageID>". Он не меняется,
// чтобы кнопки, отправленные до деплоя, продолжали работать.
type callbackData struct {
	action    callbackAction
	chatID    int64
	messageID int
}

// encodeCallback собирает строку callback data.
func encodeCallback(d callbackData) string {
	return string(d.action) + "_" + strconv.FormatInt(d.chatID, 10) + "_" + strconv.Itoa(d.messageID)
}

// decodeCallback разбирает строку callback data. Неизвестное действие,
// нечисловые ID и неверное число частей — ошибка.
func decodeCallback(s string) (callbackData, error) {
	parts := strings.Split(s, "_")
	if len(parts) != 3 {
		return callbackData{}, fmt.Errorf("callback data %q: ожидалось 3 части, получено %d", s, len(parts))
	}
	action := callbackAction(parts[0])
	if action != actionDelete && action != actionRestore {
		return callbackData{}, fmt.Errorf("callback data %q: неизвестное действие", s)
	}
	chatID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return callbackData{}, fmt.Errorf("callback data %q: chatID: %w", s, err)
	}
	messageID, err := strconv.ParseInt(parts[2], 10, 0)
	if err != nil {
		return callbackData{}, fmt.Errorf("callback data %q: messageID: %w", s, err)
	}
	return callbackData{action: action, chatID: chatID, messageID: int(messageID)}, nil
}

// toggleKeyboard возвращает клавиатуру из одной кнопки с действием d.action.
func toggleKeyboard(d callbackData) *telego.InlineKeyboardMarkup {
	text := deprecateButton
	if d.action == actionRestore {
		text = restoreButton
	}
	return &telego.InlineKeyboardMarkup{
		InlineKeyboard: [][]telego.InlineKeyboardButton{
			{
				telego.InlineKeyboardButton{
					Text:         text,
					CallbackData: encodeCallback(d),
				},
			},
		},
	}
}

// errForeignChat — callback data ссылается не на канал бота.
var errForeignChat = errors.New("chatID из callback data не совпадает с каналом бота")

// handleCallbackQuery обрабатывает нажатие кнопки «Не актуально» / «Восстановить»:
// пересобирает пост из исходного сообщения автора, редактирует его в канале
// и меняет кнопку в подтверждении на противоположное действие.
// AnswerCallbackQuery вызывается ровно один раз, в defer; текст ответа
// выставляют ветки. Ошибки API пишутся только в лог.
func (b *Bot) handleCallbackQuery(query *telego.CallbackQuery) {
	var answer string
	defer func() {
		err := b.client.AnswerCallbackQuery(context.Background(), &telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            answer,
		})
		if err != nil {
			slog.Warn("не удалось ответить на callback", "query_id", query.ID, "err", err)
		}
	}()

	d, err := decodeCallback(query.Data)
	if err == nil && d.chatID != b.channelID {
		err = fmt.Errorf("callback data %q: %w", query.Data, errForeignChat)
	}
	if err != nil {
		slog.Warn("некорректные callback data", "user_id", query.From.ID, "data", query.Data, "err", err)
		answer = callbackInvalidMessage
		return
	}

	if query.Message == nil || !query.Message.IsAccessible() {
		answer = originalMessageMissingMessage
		return
	}
	msg := query.Message.Message()

	src, ok := post.FromMessage(msg.ReplyToMessage)
	if !ok {
		answer = originalMessageMissingMessage
		return
	}

	deprecated := d.action == actionDelete
	text, ents := post.Build(src, deprecated)
	if src.Media {
		_, err = b.client.EditMessageCaption(context.Background(), &telego.EditMessageCaptionParams{
			ChatID:          telego.ChatID{ID: d.chatID},
			MessageID:       d.messageID,
			Caption:         text,
			CaptionEntities: ents,
		})
	} else {
		_, err = b.client.EditMessageText(context.Background(), &telego.EditMessageTextParams{
			ChatID:    telego.ChatID{ID: d.chatID},
			MessageID: d.messageID,
			Text:      text,
			Entities:  ents,
		})
	}
	// «message is not modified» — пост уже в нужном состоянии (кнопку нажали
	// повторно или из старого подтверждения), это успех.
	if err != nil && !isNotModified(err) {
		slog.Error("не удалось изменить пост в канале", "chat_id", d.chatID, "channel_msg_id", d.messageID, "err", err)
		answer = channelEditFailedMessage
		return
	}

	next := d
	if deprecated {
		answer = adDeprecatedMessage
		next.action = actionRestore
	} else {
		answer = adRestoredMessage
		next.action = actionDelete
	}

	// Пост в канале уже изменён, поэтому ошибка правки подтверждения
	// только логируется, а пользователь получает ответ об успехе.
	_, err = b.client.EditMessageText(context.Background(), &telego.EditMessageTextParams{
		ChatID:      telego.ChatID{ID: msg.Chat.ID},
		MessageID:   msg.MessageID,
		Text:        answer,
		ReplyMarkup: toggleKeyboard(next),
	})
	if err != nil && !isNotModified(err) {
		slog.Warn("не удалось изменить подтверждение", "chat_id", msg.Chat.ID, "message_id", msg.MessageID, "err", err)
	}
}

// isNotModified сообщает, что Telegram отклонил правку, потому что сообщение
// уже выглядит именно так.
func isNotModified(err error) bool {
	var apiErr *ta.Error
	return errors.As(err, &apiErr) && apiErr.ErrorCode == http.StatusBadRequest &&
		strings.Contains(apiErr.Description, "message is not modified")
}
