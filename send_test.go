package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tgbotapi "github.com/mymmrac/telego"
)

// TestEmptySendMediaGroupResponse: если Bot API ответил на sendMediaGroup
// пустым массивом, бот не паникует, а логирует ошибку, сообщает автору
// о неудаче и не отправляет подтверждение с кнопкой.
func TestEmptySendMediaGroupResponse(t *testing.T) {
	photo := &tgbotapi.Message{
		MessageID: 100,
		Date:      1,
		Chat:      tgbotapi.Chat{ID: testAlbumAuthorID, Type: "private"},
		From:      &tgbotapi.User{ID: testAlbumAuthorID, FirstName: "Иван"},
		Photo:     []tgbotapi.PhotoSize{{FileID: "p1"}},
		Caption:   "Продам",
	}
	tests := []struct {
		name string
		send func(b *VBBot)
	}{
		{"альбом", func(b *VBBot) { b.sendMediaGroup(pendingAlbum(MessageStateToBeSent)) }},
		{"одно фото", func(b *VBBot) { b.sendPhotoMessage(tgbotapi.Update{Message: photo}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLog(t)
			b, f := newTestBot(t, nil)
			f.respond("sendMediaGroup", "[]")

			tt.send(b)

			if out := logs.String(); !strings.Contains(out, "пустой ответ на sendMediaGroup") {
				t.Errorf("в логе нет ошибки о пустом ответе:\n%s", out)
			}
			sent := f.byMethod("sendMessage")
			if len(sent) != 1 {
				t.Fatalf("sendMessage вызван %d раз, ожидалось одно сообщение об ошибке", len(sent))
			}
			if got := jsonInt(t, sent[0].body, "chat_id"); got != testAlbumAuthorID {
				t.Errorf("сообщение об ошибке ушло в чат %d, ожидался чат автора %d", got, testAlbumAuthorID)
			}
			if _, has := sent[0].body["reply_markup"]; has {
				t.Error("автору ушло подтверждение с кнопкой, хотя публикация не подтверждена")
			}
		})
	}
}

// TestPostTooLong: объявление, которое не поместится в лимит Telegram
// (с учётом приписки и пометки «Не актуально»), не отправляется в канал,
// а автор получает PostTooLongMessage. Ровно на лимите пост публикуется.
func TestPostTooLong(t *testing.T) {
	author := &tgbotapi.User{ID: testAlbumAuthorID, FirstName: "Иван"}
	limit := func(media bool) int {
		return postTextLimit(postSource{authorID: author.ID, authorName: displayName(*author), media: media})
	}
	plain := func(text string) func(b *VBBot) {
		return func(b *VBBot) {
			b.sendPlainMessage(tgbotapi.Update{Message: &tgbotapi.Message{
				MessageID: 100, Date: 1, From: author, Text: text,
				Chat: tgbotapi.Chat{ID: testAlbumAuthorID, Type: "private"},
			}})
		}
	}
	photo := func(text string) func(b *VBBot) {
		return func(b *VBBot) {
			b.sendPhotoMessage(tgbotapi.Update{Message: &tgbotapi.Message{
				MessageID: 100, Date: 1, From: author, Caption: text,
				Photo: []tgbotapi.PhotoSize{{FileID: "p1"}},
				Chat:  tgbotapi.Chat{ID: testAlbumAuthorID, Type: "private"},
			}})
		}
	}
	album := func(text string) func(b *VBBot) {
		return func(b *VBBot) {
			mm := pendingAlbum(MessageStateToBeSent)
			mm.caption = text
			mm.originalmessageid = 100
			b.sendMediaGroup(mm)
		}
	}
	tests := []struct {
		name    string
		media   bool
		send    func(text string) func(b *VBBot)
		publish string // метод публикации в канал
	}{
		{"текст", false, plain, "sendMessage"},
		{"одно фото", true, photo, "sendMediaGroup"},
		{"альбом", true, album, "sendMediaGroup"},
	}
	for _, tt := range tests {
		n := limit(tt.media)
		t.Run(tt.name+": на лимите", func(t *testing.T) {
			captureLog(t)
			b, f := newTestBot(t, nil)
			tt.send(strings.Repeat("я", n))(b)
			if got := channelCalls(t, f, tt.publish); got != 1 {
				t.Errorf("%s в канал вызван %d раз, ожидался один", tt.publish, got)
			}
		})
		t.Run(tt.name+": длиннее лимита", func(t *testing.T) {
			captureLog(t)
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
			if got := jsonInt(t, sent[0].body, "chat_id"); got != testAlbumAuthorID {
				t.Errorf("сообщение ушло в чат %d, ожидался чат автора %d", got, testAlbumAuthorID)
			}
		})
	}
}

// channelCalls считает вызовы метода m с chat_id канала.
func channelCalls(t *testing.T, f *fakeAPI, m string) int {
	t.Helper()
	n := 0
	for _, c := range f.byMethod(m) {
		if jsonInt(t, c.body, "chat_id") == testChannelID {
			n++
		}
	}
	return n
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
		want := fmt.Sprintf(PostTooLongMessage, tt.over, tt.want)
		if got := postTooLongText(tt.over); got != want {
			t.Errorf("postTooLongText(%d) = %q, ожидалось %q", tt.over, got, want)
		}
	}
}

// TestPublishErrorHidesAPIText: при ошибке публикации автор получает понятный
// текст, а описание ошибки Telegram попадает только в лог.
func TestPublishErrorHidesAPIText(t *testing.T) {
	const apiDesc = "Bad Request: chat not found"
	author := &tgbotapi.User{ID: testAlbumAuthorID, FirstName: "Иван"}
	chat := tgbotapi.Chat{ID: testAlbumAuthorID, Type: "private"}
	tests := []struct {
		name   string
		method string
		send   func(b *VBBot)
		// wantTexts — тексты всех sendMessage автору по порядку.
		wantTexts []string
	}{
		{
			name: "текст", method: "sendMessage",
			send: func(b *VBBot) {
				b.sendPlainMessage(tgbotapi.Update{Message: &tgbotapi.Message{MessageID: 100, Date: 1, From: author, Chat: chat, Text: "Продам"}})
			},
			wantTexts: []string{PublishFailedMessage},
		},
		{
			name: "одно фото", method: "sendMediaGroup",
			send: func(b *VBBot) {
				b.sendPhotoMessage(tgbotapi.Update{Message: &tgbotapi.Message{MessageID: 100, Date: 1, From: author, Chat: chat,
					Caption: "Продам", Photo: []tgbotapi.PhotoSize{{FileID: "p1"}}}})
			},
			wantTexts: []string{PublishFailedMessage},
		},
		{
			name: "альбом", method: "sendMediaGroup",
			send:      func(b *VBBot) { b.sendMediaGroup(pendingAlbum(MessageStateToBeSent)) },
			wantTexts: []string{PublishFailedMessage},
		},
		{
			// Пост опубликован, но подтверждение с кнопкой не ушло.
			name: "подтверждение", method: "sendMessage",
			send: func(b *VBBot) {
				b.confirmNewAd(testAlbumAuthorID, 100, tgbotapi.Message{MessageID: 900, Chat: tgbotapi.Chat{ID: testChannelID}})
			},
			wantTexts: []string{AdPublishedMessage, ConfirmFailedMessage},
		},
		{
			// Служебное сообщение автору не ушло: второе сообщение тоже не дойдёт,
			// поэтому ошибка только логируется.
			name: "служебное сообщение", method: "sendMessage",
			send:      func(b *VBBot) { b.sendTextMessage(NotSubscribedMessage, testAlbumAuthorID) },
			wantTexts: []string{NotSubscribedMessage},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLog(t)
			b, f := newTestBot(t, map[string]string{tt.method: apiDesc})

			tt.send(b)

			var texts []string
			for _, c := range f.byMethod("sendMessage") {
				if jsonInt(t, c.body, "chat_id") == testAlbumAuthorID {
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
