// Package bot принимает объявления из личного чата, публикует их в канал
// и обрабатывает кнопки «Не актуально» / восстановления под ними.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/config"
)

// Bot принимает объявления и публикует их в канал. Все обновления
// обрабатываются в одной горутине Run, поэтому albums не нуждается в блокировках.
type Bot struct {
	client      *telego.Bot
	channelID   int64
	authEnabled bool
	albums      map[string]*album
	ticker      *time.Ticker
	now         func() time.Time
}

// New создаёт бота, который работает через client и публикует объявления
// в канал cfg.ChannelID.
func New(client *telego.Bot, cfg config.Config) *Bot {
	return &Bot{
		client:      client,
		channelID:   cfg.ChannelID,
		authEnabled: cfg.AuthEnabled,
		albums:      map[string]*album{},
	}
}

// ErrUpdatesClosed — канал обновлений закрылся без запроса на остановку.
var ErrUpdatesClosed = errors.New("канал обновлений закрыт без остановки бота")

// Run обрабатывает обновления до закрытия канала ch. telego закрывает его
// после отмены ctx, поэтому уже полученные обновления из буфера дочитываются
// и обрабатываются, а затем публикуются накопленные альбомы.
// Возвращает nil при остановке через ctx и ErrUpdatesClosed, если канал
// закрылся сам. Run вызывается один раз: тикер останавливается при выходе.
func (b *Bot) Run(ctx context.Context, ch <-chan telego.Update) error {
	// Тикер альбомов создаётся здесь; тесты могут задать свой заранее.
	if b.ticker == nil {
		b.ticker = time.NewTicker(albumTickInterval)
	}
	defer b.ticker.Stop()
	for {
		select {
		case update, ok := <-ch:
			if !ok {
				b.flushMediaGroups()
				if ctx.Err() != nil {
					slog.Info("бот остановлен")
					return nil
				}
				return ErrUpdatesClosed
			}
			b.dispatch(update)
		case <-b.ticker.C:
			b.tick()
		}
	}
}

// flushMediaGroups публикует все ещё не отправленные альбомы и очищает накопитель.
// Вызывается при выходе из Run, когда тикера больше не будет.
func (b *Bot) flushMediaGroups() {
	for k, v := range b.albums {
		if v.state != albumDone {
			b.publishAlbum(k, v)
		}
		delete(b.albums, k)
	}
}

// publishAlbum отправляет альбом id под recover: паника при отправке
// логируется вместе со стеком и не останавливает цикл событий. Альбом
// помечается отправленным до отправки, чтобы после паники она не повторялась
// на каждом шаге тикера.
func (b *Bot) publishAlbum(id string, a *album) {
	a.state = albumDone
	a.expiresAt = b.clock().Add(albumLateWindow)
	defer func() {
		if r := recover(); r != nil {
			slog.Error("паника при отправке альбома", "media_group_id", id, "user_id", a.authorID,
				"panic", fmt.Sprint(r), "stack", string(debug.Stack()))
		}
	}()
	b.sendMediaGroup(a)
}

// admit проверяет подписку автора и фильтры. Если сообщение не принято,
// автор получает объяснение и возвращается false.
func (b *Bot) admit(update telego.Update) bool {
	chatID := update.Message.Chat.ID
	subscribed, err := b.authByChannel(update)
	if err != nil {
		// Сырой текст ошибки API пользователю не показывается.
		slog.Warn("не удалось проверить подписку", "user_id", update.Message.From.ID, "err", err)
		b.sendTextMessage(authCheckFailedMessage, chatID)
		return false
	}
	if !subscribed {
		b.sendTextMessage(notSubscribedMessage, chatID)
		return false
	}
	if b.runFilter(update) {
		b.sendTextMessage(filtersFailedMessage, chatID)
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
func (b *Bot) addAlbumItem(update telego.Update) {
	msg := update.Message
	a, ok := b.albums[msg.MediaGroupID]
	if !ok {
		a = newAlbum(msg)
		b.albums[msg.MediaGroupID] = a
		if !b.admit(update) {
			a.rejected = true
			a.state = albumDone
			a.expiresAt = b.clock().Add(albumLateWindow)
			return
		}
	}
	if a.rejected {
		return
	}
	if a.state == albumDone {
		b.rejectLateAlbumItem(a, msg)
		return
	}
	a.addItem(msg)
	a.lastItemAt = b.clock()
	if !a.promised && a.caption != "" && len(a.fileIDs) > 0 {
		a.promised = true
		b.sendTextMessage(mediaGroupDelayMessage, msg.Chat.ID)
	}
}

// rejectLateAlbumItem обрабатывает фото альбома, который уже отправлен:
// добавить его в опубликованный пост нельзя, поэтому автор один раз на альбом
// получает albumItemLateMessage.
func (b *Bot) rejectLateAlbumItem(a *album, msg *telego.Message) {
	slog.Info("фото пришло после отправки альбома, не опубликовано",
		"user_id", msg.Chat.ID, "message_id", msg.MessageID, "media_group_id", msg.MediaGroupID)
	if a.lateNotified {
		return
	}
	a.lateNotified = true
	b.sendTextMessage(albumItemLateMessage, msg.Chat.ID)
}

// tick — один шаг тикера: отправляет альбомы, после последнего фото которых
// прошло albumQuietPeriod, и удаляет отправленные, у которых истекло окно
// опоздавших фото. Вызывается только из цикла событий Run (и из тестов).
func (b *Bot) tick() {
	now := b.clock()
	for k, v := range b.albums {
		switch {
		case v.state == albumCollecting && now.Sub(v.lastItemAt) >= albumQuietPeriod:
			b.publishAlbum(k, v)
		case v.state == albumDone && !now.Before(v.expiresAt):
			delete(b.albums, k)
		}
	}
}

// clock возвращает текущее время; тесты подменяют его через now.
func (b *Bot) clock() time.Time {
	if b.now != nil {
		return b.now()
	}
	return time.Now()
}

// dispatch обрабатывает одно обновление. Паника внутри обработчика
// логируется вместе со стеком и не останавливает цикл событий.
func (b *Bot) dispatch(update telego.Update) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("паника при обработке обновления", "update_id", update.UpdateID,
				"panic", fmt.Sprint(r), "stack", string(debug.Stack()))
		}
	}()
	if update.Message != nil {
		if !isCommand(update.Message) {
			b.handleUpdate(update)
		}
	} else if update.CallbackQuery != nil {
		b.handleCallbackQuery(update.CallbackQuery)
	}
}

func isCommand(msg *telego.Message) bool {
	return msg.Text != "" && strings.HasPrefix(msg.Text, "/")
}

func (b *Bot) confirmNewAd(chatID int64, replyToID int, sentMsg telego.Message) {
	buttonMsg := telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text:   adPublishedMessage,
		ReplyParameters: &telego.ReplyParameters{
			MessageID: replyToID,
		},
	}

	buttonMsg.ReplyMarkup = toggleKeyboard(callbackData{
		action:    actionDelete,
		chatID:    sentMsg.Chat.ID,
		messageID: sentMsg.MessageID,
	})
	if err := b.sendMessage(&buttonMsg); err != nil {
		slog.Warn("не удалось отправить подтверждение", "chat_id", chatID, "err", err)
		b.sendTextMessage(confirmFailedMessage, chatID)
	}
}

// handleUpdate обрабатывает сообщение автора. Это единственный вход для
// сообщений, поэтому дальше (authByChannel, send.go) From уже не nil.
func (b *Bot) handleUpdate(update telego.Update) {
	if update.Message.Chat.Type != telego.ChatTypePrivate {
		return
	}
	if update.Message.From == nil {
		slog.Info("сообщение без автора, пропущено", "chat_id", update.Message.Chat.ID, "message_id", update.Message.MessageID)
		return
	}
	msg := update.Message
	// Элементы альбома проверяются один раз на весь альбом, в addAlbumItem.
	if msg.MediaGroupID != "" {
		b.addAlbumItem(update)
		return
	}
	if !b.admit(update) {
		return
	}
	// Принимаются только текст и фото.
	if msg.Photo == nil && msg.Text == "" {
		slog.Info("сообщение не текст и не фото, отклонено", "user_id", msg.Chat.ID, "message_id", msg.MessageID)
		b.sendTextMessage(unsupportedMessage, msg.Chat.ID)
		return
	}
	if msg.Photo != nil {
		b.sendPhotoMessage(update)
	} else {
		b.sendPlainMessage(update)
	}
}
