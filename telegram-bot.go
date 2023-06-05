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

type MediaMessage struct {
	fileid   []string
	username string
	fullname string
	userid   int64
	caption  string
	state    MessageState
}

type MessageState int

const (
	MessageStateInit MessageState = iota
	MessageStateToBeSent
	MessageStateToDelete
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
					vbbot.sendAllPhotos(v)
					v.state = MessageStateToDelete
				} else if v.state == MessageStateToDelete {
					delete(vbbot.mediamsg, k)
				}
			}
		}
	}
}

func (vbbot *VBBot) handleUpdate(update tgbotapi.Update) {
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
				vbbot.sendBotMessage("Сообщение будет отправлено в чат через 10 секунд", update.Message.Chat.ID)
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
		msg.Text = msg.Text + "\n\n" +
			"[by: " + update.Message.From.FirstName + " " + update.Message.From.LastName +
			"](tg://user?id=" + strconv.FormatInt(update.Message.From.ID, 10) + ")"
		msg.ParseMode = tgbotapi.ModeMarkdownV2
		vbbot.sendMessageToMainChannel(msg)
	}
}

func (vbbot *VBBot) sendBotMessage(txtMsg string, chatId int64) {
	escaped := tgbotapi.EscapeText(tgbotapi.ModeMarkdownV2, txtMsg)
	botMsg := tgbotapi.NewMessage(chatId, escaped)
	vbbot.sendMessageToMainChannel(botMsg)
}

func (vbbot *VBBot) sendMessageToMainChannel(msg tgbotapi.Chattable) {
	_, err := vbbot.tgbot.Send(msg)
	if err != nil {
		log.Println(err)
	}
}

func (vbbot *VBBot) sendMediaGroupMessageToMainChannel(config tgbotapi.MediaGroupConfig) {
	_, err := vbbot.tgbot.SendMediaGroup(config)
	if err != nil {
		log.Println(err)
	}
}

func (vbbot *VBBot) sendAllPhotos(mm *MediaMessage) {

	// Изменение подписи
	caption := mm.caption + "\n\n" +
		"[by: " + mm.fullname +
		"](tg://user?id=" + strconv.FormatInt(mm.userid, 10) + ")"
	mgc := tgbotapi.MediaGroupConfig{
		ChatID: vbbot.channelId,
		Media:  make([]interface{}, 0),
	}

	for i := range mm.fileid {
		ph := mm.fileid[i]
		imp := tgbotapi.NewInputMediaPhoto(tgbotapi.FileID(ph))
		if len(mgc.Media) == 0 {
			imp.Caption = caption
			imp.ParseMode = tgbotapi.ModeMarkdownV2
		}
		mgc.Media = append(mgc.Media, imp)
	}

	vbbot.sendMediaGroupMessageToMainChannel(mgc)

}

func (vbbot *VBBot) sendPhotoMessage(update tgbotapi.Update) {
	// Получение фотографии с наибольшим размером
	photo := (update.Message.Photo)[len(update.Message.Photo)-1]

	// Запрос файла фотографии
	fileConfig := tgbotapi.FileConfig{
		FileID: photo.FileID,
	}

	// Изменение подписи
	caption := update.Message.Caption + "\n\n" +
		"[by: " + update.Message.From.FirstName + " " + update.Message.From.LastName +
		"](tg://user?id=" + strconv.FormatInt(update.Message.From.ID, 10) + ")"

	// Создание сообщения с фотографией и измененной подписью
	msg := tgbotapi.NewPhoto(vbbot.channelId, tgbotapi.FileID(fileConfig.FileID))
	msg.Caption = caption
	msg.ParseMode = tgbotapi.ModeMarkdownV2

	// Отправка сообщения с фотографией и подписью
	vbbot.sendMessageToMainChannel(msg)
}
