package main

import (
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
