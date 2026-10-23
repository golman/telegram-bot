package main

import (
	"context"
	"log"
	"runtime/debug"
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
	if err := setupLogging(); err != nil {
		log.Fatalf("failed to initialize logging: %v", err)
	}

	botToken := getEnvOrDefault("BOT_TOKEN", "")
	if botToken == "" {
		log.Fatal("BOT_TOKEN is not set")
	}
	channelID := getEnvInt64OrDefault("CHANNEL_ID", 0)
	if channelID == 0 {
		log.Fatal("CHANNEL_ID is not set or invalid")
	}

	bot, err := tgbotapi.NewBot(botToken, tgbotapi.WithDefaultDebugLogger())
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	updates, err := bot.UpdatesViaLongPolling(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	b := VBBot{
		tgbot:     bot,
		channelId: channelID,
		mediamsg:  map[string]*MediaMessage{},
		ticker:    time.NewTicker(5 * time.Second),
		cfg: VBBotCfg{
			authEnabled: getEnvBoolOrDefault("BOT_AUTH_ENABLED", true),
		},
	}
	b.Start(updates)
}

func (vbbot *VBBot) Start(ch <-chan tgbotapi.Update) {
	for {
		select {
		case update, ok := <-ch:
			if !ok {
				// Накопленные медиагруппы при выходе не отправляются:
				// это отдельный пункт TODO про graceful shutdown.
				log.Println("канал обновлений закрыт")
				return
			}
			vbbot.dispatch(update)
		case <-vbbot.ticker.C:
			vbbot.tick()
		}
	}
}

// tick — один шаг тикера: переводит каждую накопленную медиагруппу
// по состояниям Init → ToBeSent → (отправка) → ToDelete → удаление.
// Вызывается только из цикла событий Start (и из тестов).
func (vbbot *VBBot) tick() {
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

// dispatch обрабатывает одно обновление. Паника внутри обработчика
// логируется вместе со стеком и не останавливает цикл событий.
func (vbbot *VBBot) dispatch(update tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("паника при обработке обновления %d: %v\n%s", update.UpdateID, r, debug.Stack())
		}
	}()
	if update.Message != nil {
		if !IsCommand(update.Message) {
			vbbot.handleUpdate(update)
		}
	} else if update.CallbackQuery != nil {
		vbbot.handleCallbackQuery(update.CallbackQuery)
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

	buttonMsg.ReplyMarkup = toggleKeyboard(callbackData{
		action:    actionDelete,
		chatID:    sentMsg.Chat.ID,
		messageID: sentMsg.MessageID,
	})
	if err := vbbot.sendMessage(&buttonMsg); err != nil {
		vbbot.handleError(err, chatId)
	}
}

func (vbbot *VBBot) handleUpdate(update tgbotapi.Update) {
	if update.Message.Chat.ID < 0 {
		return
	}
	subscribed, err := vbbot.authByChannel(update)
	if err != nil {
		// Сырой текст ошибки API пользователю не показывается.
		log.Printf("проверка подписки пользователя %d: %v", update.Message.From.ID, err)
		vbbot.sendTextMessage(AuthCheckFailedMessage, update.Message.Chat.ID)
		return
	}
	if !subscribed {
		vbbot.sendTextMessage(NotSubscribedMessage, update.Message.Chat.ID)
		return
	}
	if vbbot.runFilter(update) {
		vbbot.sendTextMessage(FiltersFailedMessage, update.Message.Chat.ID)
		return
	}
	if update.Message.Photo != nil {
		if update.Message.MediaGroupID != "" {
			mm, ok := vbbot.mediamsg[update.Message.MediaGroupID]
			if !ok {
				mm = newMediaMessage(update.Message)
				vbbot.mediamsg[update.Message.MediaGroupID] = mm
				vbbot.sendTextMessage(MediaGroupDelayMessage, update.Message.Chat.ID)
			}
			mm.addItem(update.Message)
		} else {
			vbbot.sendPhotoMessage(update)
		}
	} else {
		vbbot.sendPlainMessage(update)
	}
}
