package main

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type VBBot struct {
	tgbot     *tgbotapi.BotAPI
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

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatal(err)
	}

	// Чтобы получать информацию о входящих обновлениях
	bot.Debug = true

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60

	updates := bot.GetUpdatesChan(updateConfig)
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

func (vbbot *VBBot) Start(ch tgbotapi.UpdatesChannel) {
	for {
		select {
		case update := <-ch:
			if update.Message != nil {
				if !update.Message.IsCommand() {
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

	var editedMsg tgbotapi.Chattable

	if len(query.Message.ReplyToMessage.Caption) == 0 {
		msg := tgbotapi.NewEditMessageText(chatID, messageID, "")
		msg.Text = "Не актуально\\.\n\n" + createObsoletteCaption(query.Message.ReplyToMessage.Text,
			query.Message.ReplyToMessage.From.FirstName+" "+query.Message.ReplyToMessage.From.LastName,
			query.Message.ReplyToMessage.From.ID)
		msg.ParseMode = tgbotapi.ModeMarkdownV2

		sentMsg, err := vbbot.tgbot.Send(msg)
		if err != nil {
			vbbot.handleError(err, query.Message.Chat.ID)
			return
		} else {
			vbbot.confirmNewAd(query.Message.From.ID, query.Message.MessageID, sentMsg)
		}

	} else {
		msg := tgbotapi.NewEditMessageCaption(chatID, messageID, "")
		msg.Caption = "Не актуально\\.\n\n" + createObsoletteCaption(query.Message.ReplyToMessage.Caption,
			query.Message.ReplyToMessage.From.FirstName+" "+query.Message.ReplyToMessage.From.LastName,
			query.Message.ReplyToMessage.From.ID)
		msg.ParseMode = tgbotapi.ModeMarkdownV2

		sentMsg, err := vbbot.tgbot.Send(msg)
		if err != nil {
			vbbot.handleError(err, query.Message.Chat.ID)
			return
		} else {
			vbbot.confirmNewAd(query.Message.From.ID, query.Message.MessageID, sentMsg)
		}
	}

	callback := tgbotapi.NewCallback(query.ID, "Ok!")
	vbbot.tgbot.Request(callback)

	editedMsg = tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, "Не актульно.")
	_, err = vbbot.tgbot.Send(editedMsg)
}

func (vbbot *VBBot) handleCallbackError(query *tgbotapi.CallbackQuery, message string) {
	callback := tgbotapi.NewCallback(query.ID, message)
	vbbot.tgbot.Request(callback)
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
					username:          update.Message.From.UserName,
					fullname:          update.Message.From.FirstName + " " + update.Message.From.LastName,
					userid:            update.Message.From.ID,
					originalmessageid: update.Message.MessageID,
					state:             MessageStateInit,
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

func createCaption(caption string, fullname string, userid int64) string {
	if strings.HasPrefix(caption, "!noescape!") {
		caption = caption[10:]
	} else {
		caption = tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, caption)
	}
	fullname = tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, fullname)
	return caption + "\n\n" +
		"by: [" + fullname + "](tg://user?id=" + strconv.FormatInt(userid, 10) + ")"
}

func createObsoletteCaption(caption string, fullname string, userid int64) string {
	if strings.HasPrefix(caption, "!noescape!") {
		caption = caption[10:]
	} else {
		caption = tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, "\\~"+caption+"\\~")
	}
	fullname = tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, fullname)
	return caption + "\n\n" +
		"by: [" + fullname + "](tg://user?id=" + strconv.FormatInt(userid, 10) + ")"
}
