package main

import (
	"log"
	"os"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type VBBot struct {
	tgbot     *tgbotapi.BotAPI
	channelId int64
	mediamsg  map[string]*MediaMessage
	ticker    *time.Ticker
}

const (
	MediaGroupDelayMessage = "Сообщение будет отправлено в чат через 10 секунд"
)

func main() {
	botToken := os.Getenv("BOT_TOKEN")
	channelIDStr := os.Getenv("CHANNEL_ID")
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

func (vbbot *VBBot) handleUpdate(update tgbotapi.Update) {
	if !vbbot.authByChannel(update) {
		vbbot.sendTextMessage("Вы не подписаны на канал", update.Message.Chat.ID)
		return
	}
	if update.Message.Photo != nil {
		if update.Message.MediaGroupID != "" {
			if _, ok := vbbot.mediamsg[update.Message.MediaGroupID]; !ok {
				vbbot.mediamsg[update.Message.MediaGroupID] = &MediaMessage{
					fileid:   []string{},
					username: update.Message.From.UserName,
					fullname: update.Message.From.FirstName + " " + update.Message.From.LastName,
					userid:   update.Message.From.ID,
					state:    MessageStateInit,
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
		msg := tgbotapi.NewMessage(vbbot.channelId, update.Message.Text)
		msg.Text = createCaption(msg.Text,
			update.Message.From.FirstName+" "+update.Message.From.LastName,
			update.Message.From.ID)
		msg.ParseMode = tgbotapi.ModeMarkdownV2
		vbbot.sendMessage(msg)
	}
}

func createCaption(caption string, fullname string, userid int64) string {
	return "```\n\n" + caption + "\n\n```" + "\n\n" +
		"[by: " + fullname +
		"](tg://user?id=" + strconv.FormatInt(userid, 10) + ")"
}
