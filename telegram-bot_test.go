package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/mymmrac/telego"
)

// runStart запускает Start и ждёт его возврата не дольше секунды.
func runStart(t *testing.T, b *VBBot, ctx context.Context, ch <-chan tgbotapi.Update) error {
	t.Helper()
	if b.ticker == nil {
		// Тикер не должен сработать во время теста: шаги tick() проверяются отдельно.
		b.ticker = time.NewTicker(time.Hour)
	}
	done := make(chan error, 1)
	go func() { done <- b.Start(ctx, ch) }()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("Start не вернулся после закрытия канала обновлений")
		return nil
	}
}

// cancelledContext возвращает уже отменённый контекст, как после SIGTERM.
func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestStartStopsOnShutdown(t *testing.T) {
	logs := captureLog(t)
	b := &VBBot{mediamsg: map[string]*MediaMessage{}}
	ch := make(chan tgbotapi.Update)
	close(ch)

	if err := runStart(t, b, cancelledContext(), ch); err != nil {
		t.Fatalf("Start при остановке вернул ошибку %v, ожидался nil", err)
	}
	if out := logs.String(); !strings.Contains(out, "бот остановлен") {
		t.Fatalf("в логе нет записи об остановке:\n%s", out)
	}
}

// TestStartWaitsForUpdatesClose: после отмены ctx Start не выходит сам,
// а ждёт, пока telego закроет канал. Иначе обновления, которые ещё лежат
// в буфере канала, были бы потеряны.
func TestStartWaitsForUpdatesClose(t *testing.T) {
	captureLog(t)
	b := &VBBot{mediamsg: map[string]*MediaMessage{}, ticker: time.NewTicker(time.Hour)}
	ch := make(chan tgbotapi.Update)
	done := make(chan error, 1)
	go func() { done <- b.Start(cancelledContext(), ch) }()

	select {
	case <-done:
		t.Fatal("Start вернулся до закрытия канала обновлений")
	case <-time.After(100 * time.Millisecond):
	}
	close(ch)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start вернул ошибку %v, ожидался nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start не вернулся после закрытия канала обновлений")
	}
}

// TestStartFailsWhenUpdatesClosedUnexpectedly: канал закрылся без остановки,
// Start возвращает ошибку, чтобы main завершился с ненулевым кодом
// и supervisor перезапустил бота.
func TestStartFailsWhenUpdatesClosedUnexpectedly(t *testing.T) {
	captureLog(t)
	b := &VBBot{mediamsg: map[string]*MediaMessage{}}
	ch := make(chan tgbotapi.Update)
	close(ch)

	err := runStart(t, b, context.Background(), ch)
	if err == nil {
		t.Fatal("Start вернул nil, ожидалась ошибка о закрытии канала обновлений")
	}
	if !strings.Contains(err.Error(), "канал обновлений закрыт") {
		t.Fatalf("ошибка %q не говорит о закрытии канала обновлений", err)
	}
}

// TestStartFlushesPendingAlbums: при остановке накопленные альбомы
// публикуются сразу, не дожидаясь тикера, а уже отправленные — нет.
func TestStartFlushesPendingAlbums(t *testing.T) {
	captureLog(t)
	b, f := newTestBot(t, nil)
	album := func(photo string, state MessageState) *MediaMessage {
		return &MediaMessage{
			fileid:   []string{photo},
			fullname: "Иван",
			userid:   testAlbumAuthorID,
			caption:  "Продам",
			state:    state,
		}
	}
	b.mediamsg = map[string]*MediaMessage{
		"init":    album("p-init", MessageStateInit),
		"tobe":    album("p-tobe", MessageStateToBeSent),
		"already": album("p-already", MessageStateToDelete),
	}
	ch := make(chan tgbotapi.Update)
	close(ch)

	if err := runStart(t, b, cancelledContext(), ch); err != nil {
		t.Fatalf("Start вернул ошибку: %v", err)
	}

	sent := map[string]bool{}
	for _, c := range f.byMethod("sendMediaGroup") {
		media := jsonField[[]any](t, c.body, "media")
		sent[jsonString(t, jsonIndex[map[string]any](t, media, 0), "media")] = true
	}
	want := map[string]bool{"p-init": true, "p-tobe": true}
	if !reflect.DeepEqual(sent, want) {
		t.Errorf("опубликованы альбомы %v, ожидались %v", sent, want)
	}
	if len(b.mediamsg) != 0 {
		t.Errorf("после остановки осталось медиагрупп: %d", len(b.mediamsg))
	}
}

// TestStartDrainsBufferedUpdates: обновления, которые telego уже положил
// в буфер канала, обрабатываются до выхода. Telegram считает их доставленными,
// и без обработки они бы пропали. Альбом из буфера публикуется при остановке.
func TestStartDrainsBufferedUpdates(t *testing.T) {
	captureLog(t)
	b, f := newTestBot(t, nil)
	b.mediamsg = map[string]*MediaMessage{}
	ch := make(chan tgbotapi.Update, 2)
	ch <- tgbotapi.Update{UpdateID: 1, Message: albumItem(100, "p1", "Продам", nil)}
	ch <- tgbotapi.Update{UpdateID: 2, Message: albumItem(101, "p2", "", nil)}
	close(ch)

	if err := runStart(t, b, cancelledContext(), ch); err != nil {
		t.Fatalf("Start вернул ошибку: %v", err)
	}

	groups := f.byMethod("sendMediaGroup")
	if len(groups) != 1 {
		t.Fatalf("sendMediaGroup вызван %d раз, ожидался один", len(groups))
	}
	if media := jsonField[[]any](t, groups[0].body, "media"); len(media) != 2 {
		t.Errorf("в альбоме %d медиа, ожидалось 2", len(media))
	}
}

// TestDispatchRecoversFromPanic проверяет, что паника в обработчике
// не выходит из dispatch и попадает в лог со стеком.
//
// Источник паники: у VBBot нет клиента (tgbot == nil), а handleCallbackQuery
// в defer всегда вызывает tgbot.AnswerCallbackQuery. Пустые callback data
// не разбираются, обработчик сразу выходит, и defer разыменовывает nil-клиент.
// Если этот путь перестанет паниковать (например, появится проверка
// tgbot != nil) и тест упадёт с «в логе нет паники», замени источник
// паники в тесте, а не чини dispatch.
//
// Запросов к API здесь быть не может: клиента нет, поэтому фейковый API
// не нужен.
func TestDispatchRecoversFromPanic(t *testing.T) {
	logs := captureLog(t)
	b := &VBBot{}
	update := tgbotapi.Update{UpdateID: 7, CallbackQuery: &tgbotapi.CallbackQuery{ID: "q"}}

	b.dispatch(update)

	// Паника должна была произойти и попасть в лог вместе со стеком,
	// иначе тест прошёл бы, даже если обработчик вышел без паники.
	out := logs.String()
	if !strings.Contains(out, "паника при обработке обновления 7") || !strings.Contains(out, "goroutine") {
		t.Fatalf("в логе нет паники со стеком:\n%s", out)
	}
}

// pendingAlbum — накопленный альбом с подписью в состоянии state.
func pendingAlbum(state MessageState) *MediaMessage {
	return &MediaMessage{
		fileid:   []string{"p1"},
		fullname: "Иван",
		userid:   testAlbumAuthorID,
		caption:  "Продам",
		state:    state,
	}
}

// Источник паники в тестах ниже: у VBBot нет клиента (tgbot == nil),
// и sendMediaGroup паникует на вызове tgbot.SendMediaGroup. Если этот путь
// перестанет паниковать, замени источник паники, а не чини recover.

// TestTickRecoversFromAlbumPanic: паника при отправке альбома по тикеру
// логируется, не роняет процесс и не повторяется на следующих тиках.
func TestTickRecoversFromAlbumPanic(t *testing.T) {
	logs := captureLog(t)
	b := &VBBot{mediamsg: map[string]*MediaMessage{"album": pendingAlbum(MessageStateToBeSent)}}

	b.tick()

	out := logs.String()
	if !strings.Contains(out, "паника при отправке альбома album") || !strings.Contains(out, "goroutine") {
		t.Fatalf("в логе нет паники со стеком:\n%s", out)
	}
	if got := b.mediamsg["album"].state; got != MessageStateToDelete {
		t.Fatalf("после паники состояние альбома %d, ожидалось ToDelete (%d)", got, MessageStateToDelete)
	}

	for i := 0; i < albumLateWindowTicks; i++ {
		b.tick()
	}
	if n := strings.Count(logs.String(), "паника при отправке альбома"); n != 1 {
		t.Errorf("паника в логе %d раз, отправка не должна повторяться", n)
	}
	if len(b.mediamsg) != 0 {
		t.Errorf("после окна ожидания осталось медиагрупп: %d", len(b.mediamsg))
	}
}

// TestFlushRecoversFromAlbumPanic: паника на одном альбоме при остановке
// не мешает очистить накопитель и не выходит из flushMediaGroups.
func TestFlushRecoversFromAlbumPanic(t *testing.T) {
	logs := captureLog(t)
	b := &VBBot{mediamsg: map[string]*MediaMessage{
		"a": pendingAlbum(MessageStateInit),
		"b": pendingAlbum(MessageStateToBeSent),
	}}

	b.flushMediaGroups()

	if n := strings.Count(logs.String(), "паника при отправке альбома"); n != 2 {
		t.Errorf("паника в логе %d раз, ожидалось по одной на каждый альбом:\n%s", n, logs.String())
	}
	if len(b.mediamsg) != 0 {
		t.Errorf("после остановки осталось медиагрупп: %d", len(b.mediamsg))
	}
}
