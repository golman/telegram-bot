package main

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/mymmrac/telego"
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

func (vbbot *VBBot) handleUpdate(update tgbotapi.Update) {
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
					fileid:   []string{},
					username: update.Message.From.Username,
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
		vbbot.sendPlainMessage(update)
	}
}
