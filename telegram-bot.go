package main

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
)

type VBBot struct {
	tgbot     *tgbotapi.Bot
	channelId int64
	mediamsg  map[string]*MediaMessage
	ticker    *time.Ticker
	cfg       VBBotCfg
}

type VBBotCfg struct {
	authEnabled bool
}

func main() {
	botToken := os.Getenv("BOT_TOKEN")
	channelIDStr := os.Getenv("CHANNEL_ID")
	authEnabledStr := os.Getenv("BOT_AUTH_ENABLED")
	channelID, err := strconv.ParseInt(channelIDStr, 10, 64)
	if err != nil {
		log.Fatal(err)
	}

	bot, err := tgbotapi.NewBot(botToken, tgbotapi.WithDefaultDebugLogger())
	if err != nil {
		log.Fatal(err)
	}

	updates, _ := bot.UpdatesViaLongPolling(nil)

	defer bot.StopLongPolling()

	b := VBBot{
		tgbot:     bot,
		channelId: channelID,
		mediamsg:  map[string]*MediaMessage{},
		ticker:    time.NewTicker(5 * time.Second),
		cfg: VBBotCfg{
			authEnabled: authEnabledStr == "true",
		},
	}
	b.Start(updates)
}

func (vbbot *VBBot) Start(ch <-chan tgbotapi.Update) {
	for {
		select {
		case update := <-ch:
			if update.Message != nil {
				if !IsCommand(update.Message) {
					vbbot.handleUpdate(update)
				}
			} else if update.CallbackQuery != nil {
				vbbot.handleCallbackQuery(update.CallbackQuery)
			}
		case <-vbbot.ticker.C:
			for k, v := range vbbot.mediamsg {
				if v.state == MessageStateInit {
					v.state = MessageStateToBeSent
				} else if v.state == MessageStateToBeSent {
					vbbot.sendMediaGroup(v)
					v.state = MessageStateToDelete
				} else if v.state == MessageStateToDelete {
					delete(vbbot.mediamsg, k)
				}
			}
		}
	}
}

func IsCommand(msg *tgbotapi.Message) bool {
	return msg.Text != "" && strings.HasPrefix(msg.Text, "/")
}

func (vbbot *VBBot) confirmNewAd(chatId int64, replyToId int, sentMsg tgbotapi.Message) {
	buttonMsg := tgbotapi.SendMessageParams{
		ChatID: tgbotapi.ChatID{ID: chatId},
		Text:   "Объявление опубликовано.\nНажми на кнопку чтобы отметить как неактуальное.",
		ReplyParameters: &tgbotapi.ReplyParameters{
			MessageID: replyToId,
		},
	}

	buttonMsg.ReplyMarkup = &tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
			{
				tgbotapi.InlineKeyboardButton{
					Text:         "Не актуально",
					CallbackData: "delete_" + strconv.Itoa(int(sentMsg.Chat.ID)) + "_" + strconv.Itoa(sentMsg.MessageID),
				},
			},
		},
	}
	if err := vbbot.sendMessage(&buttonMsg); err != nil {
		vbbot.handleError(err, chatId)
	}
}

func (vbbot *VBBot) handleUpdate(update tgbotapi.Update) {
	if update.Message.Chat.ID < 0 {
		return
	}
	if !vbbot.authByChannel(update) {
		vbbot.sendTextMessage(NotSubscribedMessage, update.Message.Chat.ID)
		return
	}
	if vbbot.runFilter(update) {
		vbbot.sendTextMessage(FiltersFailedMessage, update.Message.Chat.ID)
		return
	}
	if update.Message.Photo != nil {
		if update.Message.MediaGroupID != "" {
			if _, ok := vbbot.mediamsg[update.Message.MediaGroupID]; !ok {
				vbbot.mediamsg[update.Message.MediaGroupID] = &MediaMessage{
					fileid:            []string{},
					username:          update.Message.From.Username,
					fullname:          update.Message.From.FirstName + " " + update.Message.From.LastName,
					userid:            update.Message.From.ID,
					state:             MessageStateInit,
					entities:          update.Message.CaptionEntities,
					originalmessageid: update.Message.MessageID,
				}
				vbbot.sendTextMessage(MediaGroupDelayMessage, update.Message.Chat.ID)
			}
			vbbot.mediamsg[update.Message.MediaGroupID].fileid =
				append(vbbot.mediamsg[update.Message.MediaGroupID].fileid,
					update.Message.Photo[len(update.Message.Photo)-1].FileID)
			if update.Message.Caption != "" {
				vbbot.mediamsg[update.Message.MediaGroupID].caption = update.Message.Caption
			}
		} else {
			vbbot.sendPhotoMessage(update)
		}
	} else {
		vbbot.sendPlainMessage(update)
	}
}

func (vbbot *VBBot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	parts := strings.Split(query.Data, "_")
	if len(parts) != 3 || parts[0] != "delete" {
		return
	}

	chatID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		vbbot.handleCallbackError(query, "Invalid chat ID")
		return
	}

	messageID, err := strconv.Atoi(parts[2])
	if err != nil {
		vbbot.handleCallbackError(query, "Invalid message ID")
		return
	}
	var msg *tgbotapi.Message
	if query.Message.IsAccessible() {
		msg = query.Message.(*tgbotapi.Message)
	} else {
		vbbot.handleCallbackError(query, "Message is not accessible")
	}

	if len(msg.ReplyToMessage.Caption) == 0 {
		text, ents := telegoutil.MessageEntities(
			telegoutil.Entity("Не актуально.\n\n"),
			telegoutil.Entity(msg.ReplyToMessage.Text).Strikethrough(),
			telegoutil.Entity("\n\n by: ").Strikethrough(),
			telegoutil.Entity(msg.ReplyToMessage.From.FirstName+" "+msg.ReplyToMessage.From.LastName).TextMentionWithID(msg.ReplyToMessage.From.ID).Strikethrough())
		ents = append(ents, msg.ReplyToMessage.Entities...)
		edMsg := tgbotapi.EditMessageTextParams{
			ChatID:    tgbotapi.ChatID{ID: chatID},
			MessageID: messageID,
			Text:      text,
			Entities:  ents,
		}
		_, err := vbbot.tgbot.EditMessageText(&edMsg)
		if err != nil {
			vbbot.handleError(err, msg.Chat.ID)
			return
		}

	} else {
		text, ents := telegoutil.MessageEntities(
			telegoutil.Entity("Не актуально.\n\n"),
			telegoutil.Entity(msg.ReplyToMessage.Caption).Strikethrough(),
			telegoutil.Entity("\n\n by: ").Strikethrough(),
			telegoutil.Entity(msg.ReplyToMessage.From.FirstName+" "+msg.ReplyToMessage.From.LastName).TextMentionWithID(msg.ReplyToMessage.From.ID).Strikethrough())
		ents = append(ents, msg.ReplyToMessage.CaptionEntities...)
		edMsg := tgbotapi.EditMessageCaptionParams{
			ChatID:          tgbotapi.ChatID{ID: chatID},
			MessageID:       messageID,
			Caption:         text,
			CaptionEntities: ents,
		}
		_, err := vbbot.tgbot.EditMessageCaption(&edMsg)
		if err != nil {
			vbbot.handleError(err, msg.Chat.ID)
			return
		}
	}

	callback := tgbotapi.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            "Текст изменен!!",
	}
	vbbot.tgbot.AnswerCallbackQuery(&callback)
}

func (vbbot *VBBot) handleCallbackError(query *tgbotapi.CallbackQuery, message string) {
	callback := tgbotapi.AnswerCallbackQueryParams{
		CallbackQueryID: query.ID,
		Text:            message,
	}
	vbbot.tgbot.AnswerCallbackQuery(&callback)
}
