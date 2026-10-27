package bot

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/config"
	"github.com/golman/telegram-bot/internal/testutil"
)

// runBot запускает Run и ждёт его возврата не дольше секунды.
func runBot(t *testing.T, b *Bot, ctx context.Context, ch <-chan telego.Update) error {
	t.Helper()
	if b.ticker == nil {
		// Тикер не должен сработать во время теста: шаги tick() проверяются отдельно.
		b.ticker = time.NewTicker(time.Hour)
	}
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx, ch) }()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("Run не вернулся после закрытия канала обновлений")
		return nil
	}
}

// cancelledContext возвращает уже отменённый контекст, как после SIGTERM.
func cancelledContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// TestNew: канал и проверка подписки берутся из конфига. Без этого бот
// молча принимал бы объявления от всех, кто не подписан на канал.
func TestNew(t *testing.T) {
	for _, auth := range []bool{true, false} {
		b := New(nil, config.Config{ChannelID: -1001234567890, AuthEnabled: auth})
		if b.channelID != -1001234567890 {
			t.Errorf("получен channelID %d, ожидался -1001234567890", b.channelID)
		}
		if b.authEnabled != auth {
			t.Errorf("получен authEnabled %v, ожидался %v", b.authEnabled, auth)
		}
		if b.albums == nil {
			t.Error("получен albums nil, ожидалась пустая map")
		}
	}
}

func TestRunStopsOnShutdown(t *testing.T) {
	logs := testutil.CaptureLog(t)
	b := &Bot{albums: map[string]*album{}}
	ch := make(chan telego.Update)
	close(ch)

	if err := runBot(t, b, cancelledContext(), ch); err != nil {
		t.Fatalf("Run при остановке вернул ошибку %v, ожидался nil", err)
	}
	if out := logs.String(); !strings.Contains(out, "бот остановлен") {
		t.Fatalf("в логе нет записи об остановке:\n%s", out)
	}
}

// TestRunWaitsForUpdatesClose: после отмены ctx Run не выходит сам,
// а ждёт, пока telego закроет канал. Иначе обновления, которые ещё лежат
// в буфере канала, были бы потеряны.
func TestRunWaitsForUpdatesClose(t *testing.T) {
	testutil.CaptureLog(t)
	b := &Bot{albums: map[string]*album{}, ticker: time.NewTicker(time.Hour)}
	ch := make(chan telego.Update)
	done := make(chan error, 1)
	go func() { done <- b.Run(cancelledContext(), ch) }()

	select {
	case <-done:
		t.Fatal("Run вернулся до закрытия канала обновлений")
	case <-time.After(100 * time.Millisecond):
	}
	close(ch)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run вернул ошибку %v, ожидался nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run не вернулся после закрытия канала обновлений")
	}
}

// TestRunFailsWhenUpdatesClosedUnexpectedly: канал закрылся без остановки,
// Run возвращает ошибку, чтобы run вернул код 1
// и supervisor перезапустил бота.
func TestRunFailsWhenUpdatesClosedUnexpectedly(t *testing.T) {
	testutil.CaptureLog(t)
	b := &Bot{albums: map[string]*album{}}
	ch := make(chan telego.Update)
	close(ch)

	err := runBot(t, b, context.Background(), ch)
	if err == nil {
		t.Fatal("Run вернул nil, ожидалась ошибка о закрытии канала обновлений")
	}
	if !strings.Contains(err.Error(), "канал обновлений закрыт") {
		t.Fatalf("ошибка %q не говорит о закрытии канала обновлений", err)
	}
}

// TestRunFlushesPendingAlbums: при остановке накопленные альбомы
// публикуются сразу, не дожидаясь тикера, а уже отправленные — нет.
func TestRunFlushesPendingAlbums(t *testing.T) {
	testutil.CaptureLog(t)
	b, f := newTestBot(t, nil)
	albumWith := func(photo string, state albumState) *album {
		return &album{
			fileIDs:    []string{photo},
			authorName: "Иван",
			authorID:   testAuthorID,
			caption:    "Продам",
			state:      state,
		}
	}
	b.albums = map[string]*album{
		"init":    albumWith("p-init", albumCollecting),
		"tobe":    albumWith("p-tobe", albumCollecting),
		"already": albumWith("p-already", albumDone),
	}
	ch := make(chan telego.Update)
	close(ch)

	if err := runBot(t, b, cancelledContext(), ch); err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
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
	if len(b.albums) != 0 {
		t.Errorf("после остановки осталось медиагрупп: %d", len(b.albums))
	}
}

// TestRunTicksAlbums: Run сам делает шаги тикера, и альбом, после последнего
// фото которого прошла тишина, публикуется во время работы, а не только
// при остановке. Остальные тесты альбомов вызывают tick() напрямую.
func TestRunTicksAlbums(t *testing.T) {
	testutil.CaptureLog(t)
	b, f := newTestBot(t, nil)
	// lastItemAt не задан: тишина после последнего фото давно прошла.
	b.albums = map[string]*album{"album": pendingAlbum(albumCollecting)}
	b.ticker = time.NewTicker(10 * time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan telego.Update)
	done := make(chan error, 1)
	go func() { done <- b.Run(ctx, ch) }()

	deadline := time.After(time.Second)
	for len(f.byMethod("sendMediaGroup")) == 0 {
		select {
		case <-deadline:
			t.Fatal("получено: альбом не опубликован за секунду работы Run, ожидалась публикация по тикеру")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	close(ch)
	if err := <-done; err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
	}
	if n := len(f.byMethod("sendMediaGroup")); n != 1 {
		t.Errorf("получено sendMediaGroup %d раз, ожидался один", n)
	}
}

// TestRunTickAfterBusyLoop: цикл событий был занят дольше тишины альбома
// (например, ждал повтора 429), и за это время следующее фото альбома
// уже лежит в буфере. Шаг тикера сначала обрабатывает буфер, поэтому альбом
// не уходит без этого фото. Когда готовы и тик, и обновление, select выбирает
// случайно, поэтому сценарий повторяется: без дочитывания буфера тест падал бы
// почти наверняка.
func TestRunTickAfterBusyLoop(t *testing.T) {
	testutil.CaptureLog(t)
	for i := range 20 {
		b, f := newTestBot(t, nil)
		clock := useFakeClock(b)
		first := albumItem(100, "p1", "Продам", nil)
		a := newAlbum(first)
		a.addItem(first)
		a.promised = true
		a.lastItemAt = clock.t.Add(-20 * time.Second)
		b.albums = map[string]*album{"album": a}

		ticks := make(chan time.Time, 1)
		ticks <- clock.t
		b.ticker = &time.Ticker{C: ticks}
		ch := make(chan telego.Update, 1)
		ch <- telego.Update{Message: albumItem(101, "p2", "", nil)}
		close(ch)

		if err := runBot(t, b, cancelledContext(), ch); err != nil {
			t.Fatalf("прогон %d: Run вернул ошибку: %v", i, err)
		}
		groups := f.byMethod("sendMediaGroup")
		if len(groups) != 1 {
			t.Fatalf("прогон %d: получено sendMediaGroup %d раз, ожидался один", i, len(groups))
		}
		if n := len(jsonField[[]any](t, groups[0].body, "media")); n != 2 {
			t.Fatalf("прогон %d: получен альбом из %d фото, ожидалось 2: фото из буфера не должно отставать от тика", i, n)
		}
	}
}

// TestRunDrainsBufferedUpdates: обновления, которые telego уже положил
// в буфер канала, обрабатываются до выхода. Telegram считает их доставленными,
// и без обработки они бы пропали. Альбом из буфера публикуется при остановке.
func TestRunDrainsBufferedUpdates(t *testing.T) {
	testutil.CaptureLog(t)
	b, f := newTestBot(t, nil)
	b.albums = map[string]*album{}
	ch := make(chan telego.Update, 2)
	ch <- telego.Update{UpdateID: 1, Message: albumItem(100, "p1", "Продам", nil)}
	ch <- telego.Update{UpdateID: 2, Message: albumItem(101, "p2", "", nil)}
	close(ch)

	if err := runBot(t, b, cancelledContext(), ch); err != nil {
		t.Fatalf("Run вернул ошибку: %v", err)
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
// Источник паники: у Bot нет клиента (client == nil), а handleCallbackQuery
// в defer всегда вызывает client.AnswerCallbackQuery. Пустые callback data
// не разбираются, обработчик сразу выходит, и defer разыменовывает nil-клиент.
// Если этот путь перестанет паниковать (например, появится проверка
// client != nil) и тест упадёт с «в логе нет паники», замени источник
// паники в тесте, а не чини dispatch.
//
// Запросов к API здесь быть не может: клиента нет, поэтому фейковый API
// не нужен.
func TestDispatchRecoversFromPanic(t *testing.T) {
	logs := testutil.CaptureLog(t)
	b := &Bot{}
	update := telego.Update{UpdateID: 7, CallbackQuery: &telego.CallbackQuery{ID: "q"}}

	b.dispatch(update)

	// Паника должна была произойти и попасть в лог вместе со стеком,
	// иначе тест прошёл бы, даже если обработчик вышел без паники.
	out := logs.String()
	if !strings.Contains(out, `msg="паника при обработке обновления"`) || !strings.Contains(out, "update_id=7") || !strings.Contains(out, "goroutine") {
		t.Fatalf("в логе нет паники со стеком:\n%s", out)
	}
}

// Источник паники в тестах ниже: у Bot нет клиента (client == nil),
// и sendMediaGroup паникует на вызове client.SendMediaGroup. Если этот путь
// перестанет паниковать, замени источник паники, а не чини recover.

// TestTickRecoversFromAlbumPanic: паника при отправке альбома по тикеру
// логируется, не роняет процесс и не повторяется на следующих тиках.
func TestTickRecoversFromAlbumPanic(t *testing.T) {
	logs := testutil.CaptureLog(t)
	b := &Bot{albums: map[string]*album{"album": pendingAlbum(albumCollecting)}}
	clock := useFakeClock(b)

	b.tick()

	out := logs.String()
	if !strings.Contains(out, `msg="паника при отправке альбома"`) || !strings.Contains(out, "media_group_id=album") || !strings.Contains(out, "goroutine") {
		t.Fatalf("в логе нет паники со стеком:\n%s", out)
	}
	if got := b.albums["album"].state; got != albumDone {
		t.Fatalf("после паники состояние альбома %d, ожидалось albumDone (%d)", got, albumDone)
	}

	for elapsed := time.Duration(0); elapsed < albumLateWindow; elapsed += albumTickInterval {
		clock.wait(b, albumTickInterval)
	}
	if n := strings.Count(logs.String(), "паника при отправке альбома"); n != 1 {
		t.Errorf("паника в логе %d раз, отправка не должна повторяться", n)
	}
	if len(b.albums) != 0 {
		t.Errorf("после окна ожидания осталось медиагрупп: %d", len(b.albums))
	}
}

// TestFlushRecoversFromAlbumPanic: паника на одном альбоме при остановке
// не мешает очистить накопитель и не выходит из flushMediaGroups.
func TestFlushRecoversFromAlbumPanic(t *testing.T) {
	logs := testutil.CaptureLog(t)
	b := &Bot{albums: map[string]*album{
		"a": pendingAlbum(albumCollecting),
		"b": pendingAlbum(albumCollecting),
	}}

	b.flushMediaGroups()

	if n := strings.Count(logs.String(), "паника при отправке альбома"); n != 2 {
		t.Errorf("паника в логе %d раз, ожидалось по одной на каждый альбом:\n%s", n, logs.String())
	}
	if len(b.albums) != 0 {
		t.Errorf("после остановки осталось медиагрупп: %d", len(b.albums))
	}
}
