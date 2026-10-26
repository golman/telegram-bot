package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	tgbotapi "github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
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
func toggleKeyboard(d callbackData) *tgbotapi.InlineKeyboardMarkup {
	text := DeprecateButton
	if d.action == actionRestore {
		text = RestoreButton
	}
	return &tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
			{
				tgbotapi.InlineKeyboardButton{
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
func (vbbot *VBBot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	var answer string
	defer func() {
		err := vbbot.tgbot.AnswerCallbackQuery(context.Background(), &tgbotapi.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            answer,
		})
		if err != nil {
			log.Printf("ответ на callback %q: %v", query.ID, err)
		}
	}()

	d, err := decodeCallback(query.Data)
	if err == nil && d.chatID != vbbot.channelId {
		err = fmt.Errorf("callback data %q: %w", query.Data, errForeignChat)
	}
	if err != nil {
		log.Println(err)
		answer = CallbackInvalidMessage
		return
	}

	if query.Message == nil || !query.Message.IsAccessible() {
		answer = OriginalMessageMissingMessage
		return
	}
	msg := query.Message.Message()

	src, ok := postSourceFromMessage(msg.ReplyToMessage)
	if !ok {
		answer = OriginalMessageMissingMessage
		return
	}

	deprecated := d.action == actionDelete
	text, ents := buildPost(src, deprecated)
	if src.media {
		_, err = vbbot.tgbot.EditMessageCaption(context.Background(), &tgbotapi.EditMessageCaptionParams{
			ChatID:          tgbotapi.ChatID{ID: d.chatID},
			MessageID:       d.messageID,
			Caption:         text,
			CaptionEntities: ents,
		})
	} else {
		_, err = vbbot.tgbot.EditMessageText(context.Background(), &tgbotapi.EditMessageTextParams{
			ChatID:    tgbotapi.ChatID{ID: d.chatID},
			MessageID: d.messageID,
			Text:      text,
			Entities:  ents,
		})
	}
	// «message is not modified» — пост уже в нужном состоянии (кнопку нажали
	// повторно или из старого подтверждения), это успех.
	if err != nil && !isNotModified(err) {
		log.Printf("редактирование поста %d в канале %d: %v", d.messageID, d.chatID, err)
		answer = ChannelEditFailedMessage
		return
	}

	next := d
	if deprecated {
		answer = AdDeprecatedMessage
		next.action = actionRestore
	} else {
		answer = AdRestoredMessage
		next.action = actionDelete
	}

	// Пост в канале уже изменён, поэтому ошибка правки подтверждения
	// только логируется, а пользователь получает ответ об успехе.
	_, err = vbbot.tgbot.EditMessageText(context.Background(), &tgbotapi.EditMessageTextParams{
		ChatID:      tgbotapi.ChatID{ID: msg.Chat.ID},
		MessageID:   msg.MessageID,
		Text:        answer,
		ReplyMarkup: toggleKeyboard(next),
	})
	if err != nil && !isNotModified(err) {
		log.Printf("редактирование подтверждения %d в чате %d: %v", msg.MessageID, msg.Chat.ID, err)
	}
}

// isNotModified сообщает, что Telegram отклонил правку, потому что сообщение
// уже выглядит именно так.
func isNotModified(err error) bool {
	var apiErr *ta.Error
	return errors.As(err, &apiErr) && apiErr.ErrorCode == http.StatusBadRequest &&
		strings.Contains(apiErr.Description, "message is not modified")
}
