package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	now       func() time.Time
	cfg       VBBotCfg
}

type VBBotCfg struct {
	authEnabled bool
}

// fatal логирует ошибку запуска и завершает процесс с кодом 1.
func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

func main() {
	debug, err := setupLogging()
	if err != nil {
		fatal("не удалось настроить логирование", "err", err)
	}

	botToken := getEnvOrDefault("BOT_TOKEN", "")
	if botToken == "" {
		fatal("не задан BOT_TOKEN")
	}
	channelID := getEnvInt64OrDefault("CHANNEL_ID", 0)
	if channelID == 0 {
		fatal("не задан или некорректен CHANNEL_ID")
	}

	opts := append(botOptions(botToken, debug), tgbotapi.WithAPICaller(newAPICaller()))
	bot, err := tgbotapi.NewBot(botToken, opts...)
	if err != nil {
		fatal("не удалось создать клиент Bot API", "err", err)
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
		fatal("не удалось запустить long polling", "err", err)
	}

	b := VBBot{
		tgbot:     bot,
		channelId: channelID,
		mediamsg:  map[string]*MediaMessage{},
		ticker:    time.NewTicker(albumTickInterval),
		cfg: VBBotCfg{
			authEnabled: getEnvBoolOrDefault("BOT_AUTH_ENABLED", true),
		},
	}
	if err := b.Start(ctx, updates); err != nil {
		stop()
		fatal("бот остановился с ошибкой", "err", err)
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
					slog.Info("бот остановлен")
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
	mm.expiresAt = vbbot.clock().Add(albumLateWindow)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("паника при отправке альбома", "media_group_id", id, "user_id", mm.userid,
				"panic", fmt.Sprint(r), "stack", string(debug.Stack()))
		}
	}()
	vbbot.sendMediaGroup(mm)
}

// admit проверяет подписку автора и фильтры. Если сообщение не принято,
// автор получает объяснение и возвращается false.
func (vbbot *VBBot) admit(update tgbotapi.Update) bool {
	chatID := update.Message.Chat.ID
	subscribed, err := vbbot.authByChannel(update)
	if err != nil {
		// Сырой текст ошибки API пользователю не показывается.
		slog.Warn("не удалось проверить подписку", "user_id", update.Message.From.ID, "err", err)
		vbbot.sendTextMessage(AuthCheckFailedMessage, chatID)
		return false
	}
	if !subscribed {
		vbbot.sendTextMessage(NotSubscribedMessage, chatID)
		return false
	}
	if vbbot.runFilter(update) {
		vbbot.sendTextMessage(FiltersFailedMessage, chatID)
		return false
	}
	return true
}

// addAlbumItem добавляет элемент альбома в накопитель. Подписка и фильтры
// проверяются один раз, на первом пришедшем элементе: при отказе автор
// получает одно сообщение, а накопитель помечается отклонённым и молча
// отбрасывает остальные элементы, пока не истечёт albumLateWindow.
// Видео и файлы собираются вместе с фото, чтобы не потерять подпись,
// но не публикуются. «Будет отправлено» обещается, как только у альбома
// есть и фото, и подпись.
func (vbbot *VBBot) addAlbumItem(update tgbotapi.Update) {
	msg := update.Message
	mm, ok := vbbot.mediamsg[msg.MediaGroupID]
	if !ok {
		mm = newMediaMessage(msg)
		vbbot.mediamsg[msg.MediaGroupID] = mm
		if !vbbot.admit(update) {
			mm.rejected = true
			mm.state = MessageStateToDelete
			mm.expiresAt = vbbot.clock().Add(albumLateWindow)
			return
		}
	}
	if mm.rejected {
		return
	}
	if mm.state == MessageStateToDelete {
		vbbot.rejectLateAlbumItem(mm, msg)
		return
	}
	mm.addItem(msg)
	mm.lastItemAt = vbbot.clock()
	if !mm.promised && mm.caption != "" && len(mm.fileid) > 0 {
		mm.promised = true
		vbbot.sendTextMessage(MediaGroupDelayMessage, msg.Chat.ID)
	}
}

// rejectLateAlbumItem обрабатывает фото альбома, который уже отправлен:
// добавить его в опубликованный пост нельзя, поэтому автор один раз на альбом
// получает AlbumItemLateMessage.
func (vbbot *VBBot) rejectLateAlbumItem(mm *MediaMessage, msg *tgbotapi.Message) {
	slog.Info("фото пришло после отправки альбома, не опубликовано",
		"user_id", msg.Chat.ID, "message_id", msg.MessageID, "media_group_id", msg.MediaGroupID)
	if mm.lateNotified {
		return
	}
	mm.lateNotified = true
	vbbot.sendTextMessage(AlbumItemLateMessage, msg.Chat.ID)
}

// tick — один шаг тикера: отправляет альбомы, после последнего фото которых
// прошло albumQuietPeriod, и удаляет отправленные, у которых истекло окно
// опоздавших фото. Вызывается только из цикла событий Start (и из тестов).
func (vbbot *VBBot) tick() {
	now := vbbot.clock()
	for k, v := range vbbot.mediamsg {
		switch {
		case v.state == MessageStateInit && now.Sub(v.lastItemAt) >= albumQuietPeriod:
			vbbot.publishAlbum(k, v)
		case v.state == MessageStateToDelete && !now.Before(v.expiresAt):
			delete(vbbot.mediamsg, k)
		}
	}
}

// clock возвращает текущее время; тесты подменяют его через now.
func (vbbot *VBBot) clock() time.Time {
	if vbbot.now != nil {
		return vbbot.now()
	}
	return time.Now()
}

// dispatch обрабатывает одно обновление. Паника внутри обработчика
// логируется вместе со стеком и не останавливает цикл событий.
func (vbbot *VBBot) dispatch(update tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("паника при обработке обновления", "update_id", update.UpdateID,
				"panic", fmt.Sprint(r), "stack", string(debug.Stack()))
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
		slog.Warn("не удалось отправить подтверждение", "chat_id", chatId, "err", err)
		vbbot.sendTextMessage(ConfirmFailedMessage, chatId)
	}
}

// handleUpdate обрабатывает сообщение автора. Это единственный вход для
// сообщений, поэтому дальше (authByChannel, send.go) From уже не nil.
func (vbbot *VBBot) handleUpdate(update tgbotapi.Update) {
	if update.Message.Chat.Type != tgbotapi.ChatTypePrivate {
		return
	}
	if update.Message.From == nil {
		slog.Info("сообщение без автора, пропущено", "chat_id", update.Message.Chat.ID, "message_id", update.Message.MessageID)
		return
	}
	msg := update.Message
	// Элементы альбома проверяются один раз на весь альбом, в addAlbumItem.
	if msg.MediaGroupID != "" {
		vbbot.addAlbumItem(update)
		return
	}
	if !vbbot.admit(update) {
		return
	}
	// Принимаются только текст и фото.
	if msg.Photo == nil && msg.Text == "" {
		slog.Info("сообщение не текст и не фото, отклонено", "user_id", msg.Chat.ID, "message_id", msg.MessageID)
		vbbot.sendTextMessage(UnsupportedMessage, msg.Chat.ID)
		return
	}
	if msg.Photo != nil {
		vbbot.sendPhotoMessage(update)
	} else {
		vbbot.sendPlainMessage(update)
	}
}
