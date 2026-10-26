package main

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	tgbotapi "github.com/mymmrac/telego"
)

// pressButton нажимает кнопку с callback data data под подтверждением,
// которое отвечает на reply, и возвращает текст ответа на callback.
func pressButton(t *testing.T, vb *VBBot, f *fakeAPI, id, data string, reply *tgbotapi.Message) string {
	t.Helper()
	before := len(f.byMethod("answerCallbackQuery"))
	vb.handleCallbackQuery(&tgbotapi.CallbackQuery{
		ID:      id,
		From:    tgbotapi.User{ID: testAuthorID, FirstName: "Иван"},
		Data:    data,
		Message: confirmMsg(reply),
	})
	answers := f.byMethod("answerCallbackQuery")
	if len(answers) != before+1 {
		t.Fatalf("answerCallbackQuery вызван %d раз, ожидался %d", len(answers), before+1)
	}
	return jsonString(t, answers[len(answers)-1].body, "text")
}

// TestAlbumEndToEnd проходит путь альбома из двух фото: элементы приходят
// отдельными обновлениями, тикер публикует альбом одним sendMediaGroup,
// автор получает подтверждение и нажимает «Не актуально», затем «Восстановить».
// Тикер не запускается: время двигает fakeClock, шаги выполняются вызовами tick().
func TestAlbumEndToEnd(t *testing.T) {
	// «Продам» — 6 UTF-16 units, пробел — 1, 🚲 — 2 (вне BMP).
	bold := []tgbotapi.MessageEntity{{Type: "bold", Offset: 0, Length: 6}}
	italic := []tgbotapi.MessageEntity{{Type: "italic", Offset: 0, Length: 3}}

	tests := []struct {
		name       string
		cap1, cap2 string
		ent1, ent2 []tgbotapi.MessageEntity
		// wantReplyTo — индекс элемента, на который отвечает подтверждение;
		// -1, если альбом не публикуется.
		wantReplyTo int
		wantCaption string
		// wantEntities — entities подписи при публикации.
		wantEntities []tgbotapi.MessageEntity
	}{
		{
			name: "подпись на втором фото", cap2: "Продам 🚲", ent2: bold,
			wantReplyTo: 1,
			wantCaption: "Продам 🚲" + AuthorSuffix + "Иван",
			wantEntities: []tgbotapi.MessageEntity{
				{Type: "text_mention", Offset: 16, Length: 4, User: &tgbotapi.User{ID: testAuthorID}},
				{Type: "bold", Offset: 0, Length: 6},
			},
		},
		{
			name: "подпись на первом фото", cap1: "Продам 🚲", ent1: bold,
			wantReplyTo: 0,
			wantCaption: "Продам 🚲" + AuthorSuffix + "Иван",
			wantEntities: []tgbotapi.MessageEntity{
				{Type: "text_mention", Offset: 16, Length: 4, User: &tgbotapi.User{ID: testAuthorID}},
				{Type: "bold", Offset: 0, Length: 6},
			},
		},
		{
			// Поведение из комментария к addItem: побеждает последняя подпись.
			name: "подписи на обоих фото", cap1: "Первая", ent1: bold, cap2: "Вторая", ent2: italic,
			wantReplyTo: 1,
			wantCaption: "Вторая" + AuthorSuffix + "Иван",
			wantEntities: []tgbotapi.MessageEntity{
				{Type: "text_mention", Offset: 13, Length: 4, User: &tgbotapi.User{ID: testAuthorID}},
				{Type: "italic", Offset: 0, Length: 3},
			},
		},
		{name: "подписи нет", wantReplyTo: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vb, f := newTestBot(t, nil)
			vb.mediamsg = map[string]*MediaMessage{}
			clock := useFakeClock(vb)
			// Авторизацию покрывает auth_test.go.
			vb.cfg.authEnabled = false

			items := []*tgbotapi.Message{
				albumItem(100, "p1", tt.cap1, tt.ent1),
				albumItem(101, "p2", tt.cap2, tt.ent2),
			}
			for _, m := range items {
				vb.handleUpdate(tgbotapi.Update{Message: m})
			}

			// Пока тишина короче albumQuietPeriod, альбом ждёт остальные фото.
			clock.wait(vb, albumQuietPeriod-time.Second)
			if got := f.byMethod("sendMediaGroup"); len(got) != 0 {
				t.Fatalf("до конца тишины sendMediaGroup вызван %d раз, ожидалось 0", len(got))
			}
			// Тишина закончилась — альбом отправлен. Удаление накопителя
			// проверяет TestLateAlbumItem.
			clock.wait(vb, time.Second)

			sent := f.byMethod("sendMessage")
			delays := 0
			for _, c := range sent {
				if jsonString(t, c.body, "text") == MediaGroupDelayMessage {
					delays++
				}
				if got := jsonInt(t, c.body, "chat_id"); got != testAuthorID {
					t.Errorf("sendMessage в чат %d, ожидался чат автора %d", got, testAuthorID)
				}
			}
			// Альбому без подписи публикация не обещается.
			wantDelays := 1
			if tt.wantReplyTo < 0 {
				wantDelays = 0
			}
			if delays != wantDelays {
				t.Errorf("%q отправлено %d раз, ожидалось %d", MediaGroupDelayMessage, delays, wantDelays)
			}

			if tt.wantReplyTo < 0 {
				if got := f.byMethod("sendMediaGroup"); len(got) != 0 {
					t.Errorf("sendMediaGroup вызван %d раз, альбом без подписи публиковаться не должен", len(got))
				}
				if len(sent) != 1 {
					t.Fatalf("sendMessage вызван %d раз, ожидался один отказ", len(sent))
				}
				if got, want := jsonString(t, sent[0].body, "text"), EmptyMessage; got != want {
					t.Errorf("сообщение автору = %q, ожидалось %q", got, want)
				}
				return
			}

			// Публикация: одно sendMediaGroup, порядок фото сохранён,
			// подпись и entities только на первом медиа.
			groups := f.byMethod("sendMediaGroup")
			if len(groups) != 1 {
				t.Fatalf("sendMediaGroup вызван %d раз, ожидался один", len(groups))
			}
			if got := jsonInt(t, groups[0].body, "chat_id"); got != testChannelID {
				t.Errorf("альбом отправлен в чат %d, ожидался канал %d", got, testChannelID)
			}
			media := jsonField[[]any](t, groups[0].body, "media")
			if len(media) != len(items) {
				t.Fatalf("в альбоме %d медиа, ожидалось %d", len(media), len(items))
			}
			for i, want := range []string{"p1-big", "p2-big"} {
				if got := jsonString(t, jsonIndex[map[string]any](t, media, i), "media"); got != want {
					t.Errorf("медиа %d: file ID %q, ожидался %q", i, got, want)
				}
			}
			first := jsonIndex[map[string]any](t, media, 0)
			pubCaption := jsonString(t, first, "caption")
			if pubCaption != tt.wantCaption {
				t.Errorf("подпись альбома = %q, ожидалось %q", pubCaption, tt.wantCaption)
			}
			pubEntities := jsonEntities(t, first, "caption_entities")
			if !reflect.DeepEqual(pubEntities, tt.wantEntities) {
				t.Errorf("entities подписи = %+v, ожидалось %+v", pubEntities, tt.wantEntities)
			}
			second := jsonIndex[map[string]any](t, media, 1)
			for _, key := range []string{"caption", "caption_entities"} {
				if _, has := second[key]; has {
					t.Errorf("у второго медиа есть поле %q, подпись должна быть только на первом", key)
				}
			}

			// Подтверждение отвечает на элемент с подписью.
			if len(sent) != 2 {
				t.Fatalf("sendMessage вызван %d раз, ожидалось 2 (задержка и подтверждение)", len(sent))
			}
			confirm := sent[1].body
			replyID := jsonInt(t, jsonField[map[string]any](t, confirm, "reply_parameters"), "message_id")
			var reply *tgbotapi.Message
			for _, m := range items {
				if int64(m.MessageID) == replyID {
					reply = m
				}
			}
			if reply == nil {
				t.Fatalf("подтверждение отвечает на сообщение %d, такого элемента в альбоме нет", replyID)
			}
			if want := items[tt.wantReplyTo].MessageID; int(replyID) != want {
				t.Errorf("подтверждение отвечает на сообщение %d, ожидалось %d", replyID, want)
			}

			// delete: подпись с префиксом, entities автора сдвинуты на префикс.
			deleteData := jsonString(t, onlyButton(t, confirm), "callback_data")
			if got := pressButton(t, vb, f, "q-delete", deleteData, reply); got != AdDeprecatedMessage {
				t.Errorf("ответ на delete = %q, ожидалось %q", got, AdDeprecatedMessage)
			}
			captions := f.byMethod("editMessageCaption")
			if len(captions) != 1 {
				t.Fatalf("после delete editMessageCaption вызван %d раз, ожидался один", len(captions))
			}
			del := captions[0].body
			checkTarget(t, "пост", del, testChannelID, testMediaGroupMessageID)
			if got, want := jsonString(t, del, "caption"), DeprecatedPrefix+tt.wantCaption; got != want {
				t.Errorf("подпись после delete = %q, ожидалось %q", got, want)
			}
			// «Не актуально.\n\n» — 15 UTF-16 units.
			const shift = 15
			delEntities := jsonEntities(t, del, "caption_entities")
			for _, e := range pubEntities {
				e.Offset += shift
				if !containsEntity(delEntities, e) {
					t.Errorf("после delete нет сдвинутой entity %+v: %+v", e, delEntities)
				}
			}
			for _, e := range delEntities {
				shifted := e
				shifted.Offset -= shift
				if e.Type != "strikethrough" && !containsEntity(pubEntities, shifted) {
					t.Errorf("после delete лишняя entity %+v", e)
				}
			}

			// restore: подпись и entities ровно как при публикации.
			texts := f.byMethod("editMessageText")
			if len(texts) != 1 {
				t.Fatalf("после delete editMessageText вызван %d раз, ожидался один", len(texts))
			}
			restoreData := jsonString(t, onlyButton(t, texts[0].body), "callback_data")
			if got := pressButton(t, vb, f, "q-restore", restoreData, reply); got != AdRestoredMessage {
				t.Errorf("ответ на restore = %q, ожидалось %q", got, AdRestoredMessage)
			}
			captions = f.byMethod("editMessageCaption")
			if len(captions) != 2 {
				t.Fatalf("после restore editMessageCaption вызван %d раз, ожидалось 2", len(captions))
			}
			res := captions[1].body
			checkTarget(t, "пост", res, testChannelID, testMediaGroupMessageID)
			if got := jsonString(t, res, "caption"); got != pubCaption {
				t.Errorf("подпись после restore = %q, при публикации было %q", got, pubCaption)
			}
			if got := jsonEntities(t, res, "caption_entities"); !reflect.DeepEqual(got, pubEntities) {
				t.Errorf("entities после restore = %+v, при публикации было %+v", got, pubEntities)
			}
		})
	}
}

// TestLateAlbumItem: фото альбома, пришедшие после его публикации, не создают
// новый альбом без подписи. Автор один раз узнаёт, что они не попали
// в объявление. Накопитель хранится albumLateWindow после отправки
// и затем удаляется.
func TestLateAlbumItem(t *testing.T) {
	vb, f := newTestBot(t, nil)
	vb.mediamsg = map[string]*MediaMessage{}
	vb.cfg.authEnabled = false
	clock := useFakeClock(vb)

	vb.handleUpdate(tgbotapi.Update{Message: albumItem(100, "p1", "Продам", nil)})
	clock.wait(vb, albumQuietPeriod)
	if got := len(f.byMethod("sendMediaGroup")); got != 1 {
		t.Fatalf("sendMediaGroup вызван %d раз, ожидался один", got)
	}
	before := len(f.byMethod("sendMessage"))

	// Окно ожидания почти истекло, а фото всё ещё приходят.
	clock.wait(vb, albumLateWindow-time.Second)
	vb.handleUpdate(tgbotapi.Update{Message: albumItem(101, "p2", "", nil)})
	vb.handleUpdate(tgbotapi.Update{Message: albumItem(102, "p3", "", nil)})
	clock.wait(vb, time.Second)

	if got := len(f.byMethod("sendMediaGroup")); got != 1 {
		t.Errorf("sendMediaGroup вызван %d раз, опоздавшие фото публиковаться не должны", got)
	}
	sent := f.byMethod("sendMessage")[before:]
	if len(sent) != 1 {
		t.Fatalf("после опоздавших фото автору ушло %d сообщений, ожидалось одно", len(sent))
	}
	if got := jsonString(t, sent[0].body, "text"); got != AlbumItemLateMessage {
		t.Errorf("автору ушло %q, ожидалось %q", got, AlbumItemLateMessage)
	}
	if got := jsonInt(t, sent[0].body, "chat_id"); got != testAuthorID {
		t.Errorf("сообщение ушло в чат %d, ожидался чат автора %d", got, testAuthorID)
	}
	if len(vb.mediamsg) != 0 {
		t.Errorf("после окна ожидания осталось медиагрупп: %d", len(vb.mediamsg))
	}
}

// TestSentAlbumKeptForLateWindow: отправленный альбом хранится albumLateWindow
// после отправки и удаляется ровно по его истечении.
func TestSentAlbumKeptForLateWindow(t *testing.T) {
	captureLog(t)
	vb, _ := newTestBot(t, nil)
	clock := useFakeClock(vb)
	vb.mediamsg = map[string]*MediaMessage{"album": pendingAlbum(MessageStateInit)}

	vb.tick()
	clock.wait(vb, albumLateWindow-time.Second)
	if _, ok := vb.mediamsg["album"]; !ok {
		t.Fatalf("альбом удалён раньше, чем через %s после отправки", albumLateWindow)
	}
	clock.wait(vb, time.Second)
	if _, ok := vb.mediamsg["album"]; ok {
		t.Fatalf("альбом не удалён через %s после отправки", albumLateWindow)
	}
}

// TestAlbumQuietPeriod: альбом отправляется через albumQuietPeriod тишины
// после последнего фото: каждое новое фото откладывает отправку, поэтому
// медленно загружаемый альбом не разрывается на части.
func TestAlbumQuietPeriod(t *testing.T) {
	captureLog(t)
	b, f := newTestBot(t, nil)
	b.mediamsg = map[string]*MediaMessage{}
	clock := useFakeClock(b)
	sent := func() int { return len(f.byMethod("sendMediaGroup")) }

	// Фото приходят с паузами чуть меньше периода тишины.
	gap := albumQuietPeriod - time.Second
	b.handleUpdate(tgbotapi.Update{Message: albumItem(100, "p1", "Продам", nil)})
	for i, id := range []int{101, 102, 103} {
		clock.wait(b, gap)
		if n := sent(); n != 0 {
			t.Fatalf("альбом отправлен до фото %d, хотя тишины ещё не было", i+2)
		}
		b.handleUpdate(tgbotapi.Update{Message: albumItem(id, fmt.Sprintf("p%d", i+2), "", nil)})
	}
	clock.wait(b, gap)
	if n := sent(); n != 0 {
		t.Fatalf("альбом отправлен через %s после последнего фото, ожидалось через %s", gap, albumQuietPeriod)
	}
	clock.wait(b, time.Second)
	if n := sent(); n != 1 {
		t.Fatalf("sendMediaGroup вызван %d раз после тишины, ожидался один", n)
	}
	if media := jsonField[[]any](t, f.byMethod("sendMediaGroup")[0].body, "media"); len(media) != 4 {
		t.Errorf("в альбоме %d фото, ожидалось 4", len(media))
	}
}

// TestAlbumAuthOnce: подписка проверяется один раз на альбом, а не на каждое
// фото. Отказ приходит автору одним сообщением, остальные фото альбома
// молча отбрасываются; временный сбой проверки не публикует альбом без части фото.
func TestAlbumAuthOnce(t *testing.T) {
	tests := []struct {
		name          string
		fail          map[string]string
		member        string
		wantPublished bool
		wantTexts     []string
	}{
		{
			name: "участник канала", member: chatMemberJSON("member"),
			wantPublished: true, wantTexts: []string{MediaGroupDelayMessage, AdPublishedMessage},
		},
		{
			name: "не участник канала", member: chatMemberJSON("left"),
			wantTexts: []string{NotSubscribedMessage},
		},
		{
			name: "сбой проверки подписки", fail: map[string]string{"getChatMember": "Bad Request: chat not found"},
			wantTexts: []string{AuthCheckFailedMessage},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captureLog(t)
			b, f := newTestBot(t, tt.fail)
			b.mediamsg = map[string]*MediaMessage{}
			b.cfg.authEnabled = true
			clock := useFakeClock(b)
			if tt.member != "" {
				f.respond("getChatMember", tt.member)
			}

			// Подпись на втором фото, как часто бывает в клиентах Telegram.
			for i, m := range []*tgbotapi.Message{
				albumItem(100, "p1", "", nil),
				albumItem(101, "p2", "Продам", nil),
				albumItem(102, "p3", "", nil),
			} {
				b.handleUpdate(tgbotapi.Update{UpdateID: i, Message: m})
				// Между фото альбома тикер успевает сработать: отклонённый
				// альбом не должен исчезнуть и проверяться заново.
				clock.wait(b, albumTickInterval)
			}
			clock.wait(b, albumQuietPeriod)

			if got := len(f.byMethod("getChatMember")); got != 1 {
				t.Errorf("getChatMember вызван %d раз, ожидался один на альбом", got)
			}
			groups := f.byMethod("sendMediaGroup")
			if tt.wantPublished {
				if len(groups) != 1 {
					t.Fatalf("sendMediaGroup вызван %d раз, ожидался один", len(groups))
				}
				if media := jsonField[[]any](t, groups[0].body, "media"); len(media) != 3 {
					t.Errorf("в альбоме %d фото, ожидалось 3", len(media))
				}
			} else if len(groups) != 0 {
				t.Errorf("sendMediaGroup вызван %d раз, альбом публиковаться не должен", len(groups))
			}
			if got := authorTexts(t, f); !reflect.DeepEqual(got, tt.wantTexts) {
				t.Errorf("автору ушли %q, ожидалось %q", got, tt.wantTexts)
			}
		})
	}
}

// TestAlbumDelayPromise: «будет отправлено» обещается, только когда у альбома
// есть и фото, и подпись. Альбом без подписи сразу получает отказ при отправке,
// без обещания перед ним.
func TestAlbumDelayPromise(t *testing.T) {
	captureLog(t)
	b, f := newTestBot(t, nil)
	b.mediamsg = map[string]*MediaMessage{}
	clock := useFakeClock(b)
	b.handleUpdate(tgbotapi.Update{Message: albumItem(100, "p1", "", nil)})
	b.handleUpdate(tgbotapi.Update{Message: albumItem(101, "p2", "", nil)})
	if got := authorTexts(t, f); len(got) != 0 {
		t.Errorf("до подписи автору ушли %q, ожидалось ничего", got)
	}
	clock.wait(b, albumQuietPeriod)
	if got, want := authorTexts(t, f), []string{EmptyMessage}; !reflect.DeepEqual(got, want) {
		t.Errorf("автору ушли %q, ожидалось %q", got, want)
	}
}
