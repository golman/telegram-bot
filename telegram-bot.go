package main

import (
	"log"
	"os"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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

	currentMediaGroupID := ""
	for update := range updates {
		if update.Message == nil {
			continue
		}

		if update.Message.IsCommand() {
			continue
		}

		if update.Message.Photo != nil {
			if update.Message.MediaGroupID != "" {
				if update.Message.MediaGroupID != currentMediaGroupID {
					sendPhotoMessage(bot, channelID, update)

					msgText := "на данный момент бот поддерживает отправку только одной картинки с подписью. " +
						"пожалуйста, приложите остальные изображения в комментарии к посту в канале." +
						"\n" + "извините за временные неудобства."
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, msgText)
					msg.ReplyToMessageID = update.Message.MessageID

					sendMessage(bot, msg)
					currentMediaGroupID = update.Message.MediaGroupID
				}
			} else {
				sendPhotoMessage(bot, channelID, update)
			}
		} else {
			msg := tgbotapi.NewMessage(channelID, update.Message.Text)
			msg.Text = msg.Text + "\n" + "by: " +
				"by: [" + update.Message.From.FirstName + " " + update.Message.From.LastName +
				"](tg://user?id=" + strconv.FormatInt(update.Message.From.ID, 10) + ")"
			msg.ParseMode = tgbotapi.ModeMarkdownV2
			sendMessage(bot, msg)
		}
	}
}

func sendMessage(bot *tgbotapi.BotAPI, msg tgbotapi.Chattable) {
	_, err := bot.Send(msg)
	if err != nil {
		log.Println(err)
	}
}

func sendPhotoMessage(bot *tgbotapi.BotAPI, channelID int64, update tgbotapi.Update) {
	// Получение фотографии с наибольшим размером
	photo := (update.Message.Photo)[len(update.Message.Photo)-1]

	// Запрос файла фотографии
	fileConfig := tgbotapi.FileConfig{
		FileID: photo.FileID,
	}

	// Изменение подписи
	caption := update.Message.Caption + "\n" +
		"by: [" + update.Message.From.FirstName + " " + update.Message.From.LastName +
		"](tg://user?id=" + strconv.FormatInt(update.Message.From.ID, 10) + ")"

	// Создание сообщения с фотографией и измененной подписью
	msg := tgbotapi.NewPhoto(channelID, tgbotapi.FileID(fileConfig.FileID))
	msg.Caption = caption
	msg.ParseMode = tgbotapi.ModeMarkdownV2

	// Отправка сообщения с фотографией и подписью
	sendMessage(bot, msg)
}
