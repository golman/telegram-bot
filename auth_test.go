package main

import (
	"strings"
	"testing"

	tgbotapi "github.com/mymmrac/telego"
)

// authUpdate — обычное текстовое сообщение из личного чата пользователя.
func authUpdate() tgbotapi.Update {
	return tgbotapi.Update{Message: &tgbotapi.Message{
		MessageID: 10,
		From:      &tgbotapi.User{ID: testAuthorID, FirstName: "Иван"},
		Chat:      tgbotapi.Chat{ID: testAuthorID, Type: "private"},
		Text:      "Продам велосипед",
	}}
}

// wantMessage — ожидаемый sendMessage: адресат, начало текста и наличие кнопки.
type wantMessage struct {
	chat       int64
	textPrefix string
	button     bool
}

func TestHandleUpdateAuth(t *testing.T) {
	// Пост в канале и подтверждение автору — признаки успешной публикации.
	published := []wantMessage{
		{chat: testChannelID, textPrefix: "Продам велосипед" + AuthorSuffix},
		{chat: testAuthorID, textPrefix: "Объявление опубликовано.", button: true},
	}
	tests := []struct {
		name        string
		authEnabled bool
		// fail — методы, на которые фейковый API отвечает ошибкой.
		fail map[string]string
		// member — JSON ответа getChatMember, если он задан.
		member         string
		wantMemberCall int
		want           []wantMessage
		// wantLog — подстроки, которые должны попасть в лог.
		wantLog []string
	}{
		{
			name:           "ошибка getChatMember",
			authEnabled:    true,
			fail:           map[string]string{"getChatMember": "Bad Request: chat not found"},
			wantMemberCall: 1,
			want:           []wantMessage{{chat: testAuthorID, textPrefix: AuthCheckFailedMessage}},
			wantLog:        []string{"проверка подписки пользователя 42: ", "chat not found"},
		},
		{
			name:           "пользователь не участник канала",
			authEnabled:    true,
			member:         chatMemberJSON("left"),
			wantMemberCall: 1,
			want:           []wantMessage{{chat: testAuthorID, textPrefix: NotSubscribedMessage}},
		},
		{
			name:           "участник канала",
			authEnabled:    true,
			member:         chatMemberJSON("member"),
			wantMemberCall: 1,
			want:           published,
		},
		{
			name:           "авторизация выключена",
			authEnabled:    false,
			wantMemberCall: 0,
			want:           published,
		},
	}
	serviceTexts := []string{AuthCheckFailedMessage, NotSubscribedMessage, FiltersFailedMessage}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLog(t)
			b, api := newTestBot(t, tt.fail)
			b.cfg.authEnabled = tt.authEnabled
			if tt.member != "" {
				api.respond("getChatMember", tt.member)
			}

			// До исправления log.Fatalln здесь завершал тестовый бинарник.
			b.handleUpdate(authUpdate())

			if got := len(api.byMethod("getChatMember")); got != tt.wantMemberCall {
				t.Fatalf("вызовов getChatMember %d, ожидалось %d", got, tt.wantMemberCall)
			}
			sent := api.byMethod("sendMessage")
			if len(sent) != len(tt.want) {
				t.Fatalf("sendMessage %d, ожидалось %d: %v", len(sent), len(tt.want), sent)
			}
			for i, w := range tt.want {
				body := sent[i].body
				if chat := jsonInt(t, body, "chat_id"); chat != w.chat {
					t.Errorf("sendMessage %d: чат %d, ожидался %d", i, chat, w.chat)
				}
				text := jsonString(t, body, "text")
				if !strings.HasPrefix(text, w.textPrefix) {
					t.Errorf("sendMessage %d: текст %q, ожидалось начало %q", i, text, w.textPrefix)
				}
				// Служебный текст уходит только тогда, когда он и ожидается.
				for _, s := range serviceTexts {
					if text == s && w.textPrefix != s {
						t.Errorf("sendMessage %d: неожиданный служебный текст %q", i, s)
					}
				}
				if _, has := body["reply_markup"]; has != w.button {
					t.Errorf("sendMessage %d: кнопка %v, ожидалась %v", i, has, w.button)
				} else if has {
					btn := onlyButton(t, body)
					if got := jsonString(t, btn, "text"); got != DeprecateButton {
						t.Errorf("sendMessage %d: кнопка %q, ожидалась %q", i, got, DeprecateButton)
					}
				}
			}
			// Альбомом в канал тоже ничего не уходит: сообщение текстовое.
			if got := api.byMethod("sendMediaGroup"); len(got) != 0 {
				t.Errorf("не ожидалось sendMediaGroup, получено %v", got)
			}
			for _, l := range tt.wantLog {
				if !strings.Contains(logs.String(), l) {
					t.Errorf("в логе нет %q:\n%s", l, logs.String())
				}
			}
		})
	}
}
