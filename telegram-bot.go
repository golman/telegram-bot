package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
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

	bot, err := tgbotapi.NewBot(botToken, tgbotapi.WithDefaultDebugLogger(),
		tgbotapi.WithAPICaller(newAPICaller()))
	if err != nil {
		log.Fatal(err)
	}

	// SIGINT/SIGTERM отменяют ctx: telego прекращает long polling и закрывает
	// канал обновлений, после чего Start дообрабатывает буфер и выходит.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		// Повторный сигнал снова завершает процесс сразу, если остановка зависла.
		stop()
	}()

	updates, err := bot.UpdatesViaLongPolling(ctx, &tgbotapi.GetUpdatesParams{Timeout: longPollTimeoutSeconds})
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
	if err := b.Start(ctx, updates); err != nil {
		stop()
		log.Fatal(err)
	}
}

// errUpdatesClosed — канал обновлений закрылся без запроса на остановку.
var errUpdatesClosed = errors.New("канал обновлений закрыт без остановки бота")

// Start обрабатывает обновления до закрытия канала ch. telego закрывает его
// после отмены ctx, поэтому уже полученные обновления из буфера дочитываются
// и обрабатываются, а затем публикуются накопленные альбомы.
// Возвращает nil при остановке через ctx и errUpdatesClosed, если канал
// закрылся сам.
func (vbbot *VBBot) Start(ctx context.Context, ch <-chan tgbotapi.Update) error {
	defer vbbot.ticker.Stop()
	for {
		select {
		case update, ok := <-ch:
			if !ok {
				vbbot.flushMediaGroups()
				if ctx.Err() != nil {
					log.Println("бот остановлен")
					return nil
				}
				return errUpdatesClosed
			}
			vbbot.dispatch(update)
		case <-vbbot.ticker.C:
			vbbot.tick()
		}
	}
}

// flushMediaGroups публикует все ещё не отправленные альбомы и очищает накопитель.
// Вызывается при выходе из Start, когда тикера больше не будет.
func (vbbot *VBBot) flushMediaGroups() {
	for k, v := range vbbot.mediamsg {
		if v.state != MessageStateToDelete {
			vbbot.publishAlbum(k, v)
		}
		delete(vbbot.mediamsg, k)
	}
}

// publishAlbum отправляет альбом id под recover: паника при отправке
// логируется вместе со стеком и не останавливает цикл событий. Альбом
// помечается отправленным до отправки, чтобы после паники она не повторялась
// на каждом шаге тикера.
func (vbbot *VBBot) publishAlbum(id string, mm *MediaMessage) {
	mm.state = MessageStateToDelete
	mm.ticksLeft = albumLateWindowTicks
	defer func() {
		if r := recover(); r != nil {
			log.Printf("паника при отправке альбома %s: %v\n%s", id, r, debug.Stack())
		}
	}()
	vbbot.sendMediaGroup(mm)
}

// rejectLateAlbumItem обрабатывает фото альбома, который уже отправлен:
// добавить его в опубликованный пост нельзя, поэтому автор один раз на альбом
// получает AlbumItemLateMessage.
func (vbbot *VBBot) rejectLateAlbumItem(mm *MediaMessage, msg *tgbotapi.Message) {
	log.Printf("фото %d пришло после отправки альбома %s и не опубликовано", msg.MessageID, msg.MediaGroupID)
	if mm.lateNotified {
		return
	}
	mm.lateNotified = true
	vbbot.sendTextMessage(AlbumItemLateMessage, msg.Chat.ID)
}

// tick — один шаг тикера: переводит каждую накопленную медиагруппу
// по состояниям Init → ToBeSent → (отправка) → ToDelete → (albumLateWindowTicks
// шагов ожидания опоздавших фото) → удаление.
// Вызывается только из цикла событий Start (и из тестов).
func (vbbot *VBBot) tick() {
	for k, v := range vbbot.mediamsg {
		if v.state == MessageStateInit {
			v.state = MessageStateToBeSent
		} else if v.state == MessageStateToBeSent {
			vbbot.publishAlbum(k, v)
		} else if v.state == MessageStateToDelete {
			v.ticksLeft--
			if v.ticksLeft <= 0 {
				delete(vbbot.mediamsg, k)
			}
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
		Text:   AdPublishedMessage,
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
		log.Printf("не удалось отправить подтверждение в чат %d: %v", chatId, err)
		vbbot.sendTextMessage(ConfirmFailedMessage, chatId)
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
			if mm.state == MessageStateToDelete {
				vbbot.rejectLateAlbumItem(mm, update.Message)
				return
			}
			mm.addItem(update.Message)
		} else {
			vbbot.sendPhotoMessage(update)
		}
	} else {
		vbbot.sendPlainMessage(update)
	}
}
