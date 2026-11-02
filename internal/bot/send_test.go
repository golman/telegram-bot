package bot

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/post"
	"github.com/golman/telegram-bot/internal/testutil"
)

// TestPhotoWithoutCaption: одиночное фото без подписи не публикуется,
// автор получает объяснение, что нужна подпись.
func TestPhotoWithoutCaption(t *testing.T) {
	testutil.CaptureLog(t)
	b, f := newTestBot(t, nil)
	b.albums = map[string]*album{}
	m := authorMessage(100)
	m.Photo = []telego.PhotoSize{{FileID: "p"}}

	b.handleUpdate(telego.Update{Message: m})

	if got, want := authorTexts(t, f), []string{noCaptionMessage}; !reflect.DeepEqual(got, want) {
		t.Errorf("автору ушли %q, ожидалось %q", got, want)
	}
	if n := channelCalls(t, f, "sendMessage") + channelCalls(t, f, "sendMediaGroup") + channelCalls(t, f, "sendPhoto"); n != 0 {
		t.Errorf("в канал ушло %d запросов, ожидалось 0", n)
	}
}

// TestSinglePhotoViaSendPhoto: одиночное фото публикуется одним sendPhoto
// (самый большой размер, подпись с припиской автора), а кнопка
// подтверждения ведёт на пост из ответа sendPhoto.
func TestSinglePhotoViaSendPhoto(t *testing.T) {
	testutil.CaptureLog(t)
	b, f := newTestBot(t, nil)
	m := authorMessage(100)
	m.Photo = []telego.PhotoSize{{FileID: "p-small"}, {FileID: "p-big"}}
	m.Caption = "Продам"

	b.sendPhotoMessage(m)

	if n := len(f.byMethod("sendMediaGroup")); n != 0 {
		t.Errorf("получено sendMediaGroup %d раз, ожидалось 0", n)
	}
	sent := f.byMethod("sendPhoto")
	if len(sent) != 1 {
		t.Fatalf("получено sendPhoto %d раз, ожидался один", len(sent))
	}
	body := sent[0].body
	if got := jsonInt(t, body, "chat_id"); got != testChannelID {
		t.Errorf("получен chat_id %d, ожидался канал %d", got, testChannelID)
	}
	if got := jsonString(t, body, "photo"); got != "p-big" {
		t.Errorf("получено фото %q, ожидалось самое большое p-big", got)
	}
	if got, want := jsonString(t, body, "caption"), "Продам"+post.AuthorSuffix+"Иван"; got != want {
		t.Errorf("получена подпись %q, ожидалась %q", got, want)
	}
	mention := telego.MessageEntity{Type: telego.EntityTypeTextMention, Offset: 12, Length: 4, User: &telego.User{ID: testAuthorID}}
	if ents := jsonEntities(t, body, "caption_entities"); !containsEntity(ents, mention) {
		t.Errorf("получены caption_entities %+v, ожидалось упоминание автора %+v", ents, mention)
	}

	confirm := f.byMethod("sendMessage")
	if len(confirm) != 1 {
		t.Fatalf("получено sendMessage %d раз, ожидалось одно подтверждение", len(confirm))
	}
	want := fmt.Sprintf("delete_%d_%d", testChannelID, testPhotoMessageID)
	if got := jsonString(t, onlyButton(t, confirm[0].body), "callback_data"); got != want {
		t.Errorf("получены callback_data %q, ожидались %q", got, want)
	}
}

// TestEmptySendResponse: если Bot API ответил на sendMediaGroup пустым
// массивом или на sendPhoto пустым результатом, бот не паникует, а логирует
// ошибку, сообщает автору о неудаче и не отправляет подтверждение с кнопкой.
func TestEmptySendResponse(t *testing.T) {
	photo := authorMessage(100)
	photo.Photo = []telego.PhotoSize{{FileID: "p1"}}
	photo.Caption = "Продам"
	tests := []struct {
		name, method, result string
		send                 func(b *Bot)
	}{
		{"альбом", "sendMediaGroup", "[]", func(b *Bot) { b.sendMediaGroup(pendingAlbum(albumCollecting)) }},
		{"одно фото", "sendPhoto", "null", func(b *Bot) { b.sendPhotoMessage(photo) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := testutil.CaptureLog(t)
			b, f := newTestBot(t, nil)
			f.respond(tt.method, tt.result)

			tt.send(b)

			if out := logs.String(); !strings.Contains(out, "пустой ответ на "+tt.method) {
				t.Errorf("в логе нет ошибки о пустом ответе:\n%s", out)
			}
			sent := f.byMethod("sendMessage")
			if len(sent) != 1 {
				t.Fatalf("sendMessage вызван %d раз, ожидалось одно сообщение об ошибке", len(sent))
			}
			if got := jsonInt(t, sent[0].body, "chat_id"); got != testAuthorID {
				t.Errorf("сообщение об ошибке ушло в чат %d, ожидался чат автора %d", got, testAuthorID)
			}
			if _, has := sent[0].body["reply_markup"]; has {
				t.Error("автору ушло подтверждение с кнопкой, хотя публикация не подтверждена")
			}
		})
	}
}

// TestPostTooLong: объявление, которое не поместится в лимит Telegram
// (с учётом приписки и пометки «Не актуально»), не отправляется в канал,
// а автор получает postTooLongMessage. Ровно на лимите пост публикуется.
func TestPostTooLong(t *testing.T) {
	author := &telego.User{ID: testAuthorID, FirstName: "Иван"}
	limit := func(media bool) int {
		return post.TextLimit(post.Source{AuthorID: author.ID, AuthorName: post.DisplayName(*author), Media: media})
	}
	plain := func(text string) func(b *Bot) {
		return func(b *Bot) {
			b.sendPlainMessage(&telego.Message{
				MessageID: 100, Date: 1, From: author, Text: text,
				Chat: telego.Chat{ID: testAuthorID, Type: "private"},
			})
		}
	}
	photo := func(text string) func(b *Bot) {
		return func(b *Bot) {
			b.sendPhotoMessage(&telego.Message{
				MessageID: 100, Date: 1, From: author, Caption: text,
				Photo: []telego.PhotoSize{{FileID: "p1"}},
				Chat:  telego.Chat{ID: testAuthorID, Type: "private"},
			})
		}
	}
	sendAlbum := func(text string) func(b *Bot) {
		return func(b *Bot) {
			a := pendingAlbum(albumCollecting)
			a.caption = text
			a.replyToID = 100
			b.sendMediaGroup(a)
		}
	}
	tests := []struct {
		name    string
		media   bool
		send    func(text string) func(b *Bot)
		publish string // метод публикации в канал
	}{
		{"текст", false, plain, "sendMessage"},
		{"одно фото", true, photo, "sendPhoto"},
		{"альбом", true, sendAlbum, "sendMediaGroup"},
	}
	for _, tt := range tests {
		n := limit(tt.media)
		t.Run(tt.name+": на лимите", func(t *testing.T) {
			testutil.CaptureLog(t)
			b, f := newTestBot(t, nil)
			tt.send(strings.Repeat("я", n))(b)
			if got := channelCalls(t, f, tt.publish); got != 1 {
				t.Errorf("%s в канал вызван %d раз, ожидался один", tt.publish, got)
			}
		})
		t.Run(tt.name+": длиннее лимита", func(t *testing.T) {
			testutil.CaptureLog(t)
			b, f := newTestBot(t, nil)
			// n-1 рун «я» и 🚲 (2 UTF-16 units): в рунах ровно лимит, в UTF-16 на один больше.
			tt.send(strings.Repeat("я", n-1) + "🚲")(b)
			if got := channelCalls(t, f, tt.publish); got != 0 {
				t.Errorf("%s в канал вызван %d раз, длинный пост публиковаться не должен", tt.publish, got)
			}
			sent := f.byMethod("sendMessage")
			if len(sent) != 1 {
				t.Fatalf("sendMessage вызван %d раз, ожидалось одно сообщение автору", len(sent))
			}
			want := postTooLongText(1)
			if got := jsonString(t, sent[0].body, "text"); got != want {
				t.Errorf("автору ушло %q, ожидалось %q", got, want)
			}
			// Предупреждение отвечает на исходное сообщение с текстом объявления
			// и уходит, даже если автор это сообщение уже удалил.
			reply := jsonField[map[string]any](t, sent[0].body, "reply_parameters")
			if got := jsonInt(t, reply, "message_id"); got != 100 {
				t.Errorf("предупреждение отвечает на сообщение %d, ожидалось 100", got)
			}
			if !jsonField[bool](t, reply, "allow_sending_without_reply") {
				t.Error("allow_sending_without_reply не включён")
			}
			if got := jsonInt(t, sent[0].body, "chat_id"); got != testAuthorID {
				t.Errorf("сообщение ушло в чат %d, ожидался чат автора %d", got, testAuthorID)
			}
		})
	}
}

func TestPostTooLongText(t *testing.T) {
	tests := []struct {
		over int
		want string
	}{
		{1, "символ"}, {21, "символ"}, {101, "символ"},
		{2, "символа"}, {4, "символа"}, {22, "символа"},
		{5, "символов"}, {11, "символов"}, {12, "символов"}, {14, "символов"}, {111, "символов"}, {100, "символов"},
	}
	for _, tt := range tests {
		want := fmt.Sprintf(postTooLongMessage, tt.over, tt.want)
		if got := postTooLongText(tt.over); got != want {
			t.Errorf("postTooLongText(%d) = %q, ожидалось %q", tt.over, got, want)
		}
	}
}

// TestPublishErrorHidesAPIText: при ошибке публикации автор получает понятный
// текст, а описание ошибки Telegram попадает только в лог.
func TestPublishErrorHidesAPIText(t *testing.T) {
	const apiDesc = "Bad Request: chat not found"
	author := &telego.User{ID: testAuthorID, FirstName: "Иван"}
	chat := telego.Chat{ID: testAuthorID, Type: "private"}
	tests := []struct {
		name   string
		method string
		send   func(b *Bot)
		// wantTexts — тексты всех sendMessage автору по порядку.
		wantTexts []string
	}{
		{
			name: "текст", method: "sendMessage",
			send: func(b *Bot) {
				b.sendPlainMessage(&telego.Message{MessageID: 100, Date: 1, From: author, Chat: chat, Text: "Продам"})
			},
			wantTexts: []string{publishFailedMessage},
		},
		{
			name: "одно фото", method: "sendPhoto",
			send: func(b *Bot) {
				b.sendPhotoMessage(&telego.Message{MessageID: 100, Date: 1, From: author, Chat: chat,
					Caption: "Продам", Photo: []telego.PhotoSize{{FileID: "p1"}}})
			},
			wantTexts: []string{publishFailedMessage},
		},
		{
			name: "альбом", method: "sendMediaGroup",
			send:      func(b *Bot) { b.sendMediaGroup(pendingAlbum(albumCollecting)) },
			wantTexts: []string{publishFailedMessage},
		},
		{
			// Пост опубликован, но подтверждение с кнопкой не ушло.
			name: "подтверждение", method: "sendMessage",
			send: func(b *Bot) {
				b.confirmNewAd(testAuthorID, 100, telego.Message{MessageID: 900, Chat: telego.Chat{ID: testChannelID}})
			},
			wantTexts: []string{adPublishedMessage, confirmFailedMessage},
		},
		{
			// Служебное сообщение автору не ушло: второе сообщение тоже не дойдёт,
			// поэтому ошибка только логируется.
			name: "служебное сообщение", method: "sendMessage",
			send:      func(b *Bot) { b.sendTextMessage(notSubscribedMessage, testAuthorID) },
			wantTexts: []string{notSubscribedMessage},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := testutil.CaptureLog(t)
			b, f := newTestBot(t, map[string]string{tt.method: apiDesc})

			tt.send(b)

			var texts []string
			for _, c := range f.byMethod("sendMessage") {
				if jsonInt(t, c.body, "chat_id") == testAuthorID {
					texts = append(texts, jsonString(t, c.body, "text"))
				}
			}
			if !reflect.DeepEqual(texts, tt.wantTexts) {
				t.Errorf("автору ушли сообщения %q, ожидалось %q", texts, tt.wantTexts)
			}
			for _, text := range texts {
				if strings.Contains(text, "chat not found") {
					t.Errorf("автору ушёл сырой текст ошибки API: %q", text)
				}
			}
			if !strings.Contains(logs.String(), "chat not found") {
				t.Errorf("в логе нет описания ошибки API:\n%s", logs.String())
			}
		})
	}
}

// TestPublishUncertain: если ответа Telegram на публикацию нет (оборвалось
// соединение, таймаут) или он пустой, пост мог выйти в канал. Автор получает
// не «попробуйте позже», а просьбу сначала проверить канал, иначе повторная
// отправка дала бы дубль.
func TestPublishUncertain(t *testing.T) {
	author := &telego.User{ID: testAuthorID, FirstName: "Иван"}
	chat := telego.Chat{ID: testAuthorID, Type: "private"}
	photo := func(b *Bot) {
		b.sendPhotoMessage(&telego.Message{MessageID: 100, Date: 1, From: author, Chat: chat,
			Caption: "Продам", Photo: []telego.PhotoSize{{FileID: "p1"}}})
	}
	sendAlbum := func(b *Bot) { b.sendMediaGroup(pendingAlbum(albumCollecting)) }
	tests := []struct {
		name  string
		setup func(f *fakeAPI)
		send  func(b *Bot)
	}{
		{"одно фото: обрыв соединения", func(f *fakeAPI) { f.dropConn("sendPhoto") }, photo},
		{"альбом: обрыв соединения", func(f *fakeAPI) { f.dropConn("sendMediaGroup") }, sendAlbum},
		{"одно фото: пустой ответ", func(f *fakeAPI) { f.respond("sendPhoto", "null") }, photo},
		{"альбом: пустой ответ", func(f *fakeAPI) { f.respond("sendMediaGroup", "[]") }, sendAlbum},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := testutil.CaptureLog(t)
			b, f := newTestBot(t, nil)
			tt.setup(f)

			tt.send(b)

			if got, want := authorTexts(t, f), []string{publishUncertainMessage}; !reflect.DeepEqual(got, want) {
				t.Errorf("получены тексты автору %q, ожидались %q", got, want)
			}
			if !strings.Contains(logs.String(), `level=ERROR msg="не удалось опубликовать объявление"`) {
				t.Errorf("получено без записи об ошибке публикации, ожидалась:\n%s", logs.String())
			}
		})
	}
}

// TestEmptySendMessageResponse: если Bot API ответил на sendMessage пустым
// результатом, бот не паникует, а сообщает автору, что пост мог выйти, без кнопки.
func TestEmptySendMessageResponse(t *testing.T) {
	logs := testutil.CaptureLog(t)
	b, f := newTestBot(t, nil)
	f.respond("sendMessage", "null")
	m := authorMessage(100)
	m.Text = "Продам"

	b.sendPlainMessage(m)

	if out := logs.String(); !strings.Contains(out, "пустой ответ на sendMessage") {
		t.Errorf("получено без ошибки о пустом ответе, ожидалась:\n%s", out)
	}
	if got, want := authorTexts(t, f), []string{publishUncertainMessage}; !reflect.DeepEqual(got, want) {
		t.Errorf("получены тексты автору %q, ожидались %q", got, want)
	}
}
