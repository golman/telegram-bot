package bot

import (
	"strings"
	"testing"

	"github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/post"
	"github.com/golman/telegram-bot/internal/testutil"
)

// checkTarget проверяет, что запрос правит сообщение messageID в чате chatID.
func checkTarget(t *testing.T, what string, body map[string]any, chatID, messageID int64) {
	t.Helper()
	gotChat, gotMsg := jsonInt(t, body, "chat_id"), jsonInt(t, body, "message_id")
	if gotChat != chatID || gotMsg != messageID {
		t.Errorf("%s: правится чат %d, сообщение %d; ожидались %d и %d", what, gotChat, gotMsg, chatID, messageID)
	}
}

// checkConfirm проверяет правку подтверждения: цель, текст и кнопку.
func checkConfirm(t *testing.T, body map[string]any, wantText, wantButton, wantData string) {
	t.Helper()
	checkTarget(t, "подтверждение", body, testAuthorID, testConfirmMessageID)
	if got := jsonString(t, body, "text"); got != wantText {
		t.Errorf("текст подтверждения = %q, ожидалось %q", got, wantText)
	}
	btn := onlyButton(t, body)
	if got := jsonString(t, btn, "text"); got != wantButton {
		t.Errorf("текст кнопки = %q, ожидалось %q", got, wantButton)
	}
	if got := jsonString(t, btn, "callback_data"); got != wantData {
		t.Errorf("callback_data = %q, ожидалось %q", got, wantData)
	}
}

// notModified — так Telegram отвечает на правку, которая ничего не меняет.
const notModified = "Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message"

func TestHandleCallbackQuery(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		msg        telego.MaybeInaccessibleMessage
		fail       map[string]string
		wantAnswer string
		// wantEdits — методы правки по порядку, без answerCallbackQuery.
		wantEdits []string
		check     func(t *testing.T, f *fakeAPI)
		// quiet — в логе нет WARN и ERROR: «message is not modified» — не сбой.
		quiet bool
	}{
		{name: "неизвестное действие", data: "noop_1_2", msg: confirmMsg(textReply()), wantAnswer: callbackInvalidMessage},
		{name: "пустые данные", data: "", msg: confirmMsg(textReply()), wantAnswer: callbackInvalidMessage},
		{name: "чужой канал", data: "delete_-100_42", msg: confirmMsg(textReply()), wantAnswer: callbackInvalidMessage},
		{name: "нет сообщения", data: "delete_-1001234567890_42", msg: nil, wantAnswer: originalMessageMissingMessage},
		{
			name: "сообщение недоступно", data: "delete_-1001234567890_42",
			msg:        &telego.InaccessibleMessage{Chat: telego.Chat{ID: testAuthorID}, MessageID: testConfirmMessageID},
			wantAnswer: originalMessageMissingMessage,
		},
		{name: "исходное сообщение удалено", data: "delete_-1001234567890_42", msg: confirmMsg(nil), wantAnswer: originalMessageMissingMessage},
		{name: "у исходного нет автора", data: "delete_-1001234567890_42", msg: confirmMsg(&telego.Message{Text: "x"}), wantAnswer: originalMessageMissingMessage},
		{name: "фото без подписи", data: "delete_-1001234567890_42", msg: confirmMsg(photoReply("")), wantAnswer: originalMessageMissingMessage},
		{
			name: "текст: кнопка до деплоя, delete", data: "delete_-1001234567890_42", msg: confirmMsg(textReply()),
			wantAnswer: adDeprecatedMessage,
			wantEdits:  []string{"editMessageText", "editMessageText"},
			check: func(t *testing.T, f *fakeAPI) {
				edits := f.byMethod("editMessageText")
				body := edits[0].body
				checkTarget(t, "пост", body, testChannelID, 42)
				if got := jsonString(t, body, "text"); !strings.HasPrefix(got, post.DeprecatedPrefix) {
					t.Errorf("текст поста = %q, ожидался префикс %q", got, post.DeprecatedPrefix)
				}
				checkConfirm(t, edits[1].body, adDeprecatedMessage, restoreButton, "restore_-1001234567890_42")
			},
		},
		{
			name: "фото: restore", data: "restore_-1001234567890_43", msg: confirmMsg(photoReply("cap")),
			wantAnswer: adRestoredMessage,
			wantEdits:  []string{"editMessageCaption", "editMessageText"},
			check: func(t *testing.T, f *fakeAPI) {
				body := f.byMethod("editMessageCaption")[0].body
				checkTarget(t, "пост", body, testChannelID, 43)
				if got, want := jsonString(t, body, "caption"), "cap"+post.AuthorSuffix+"Иван"; got != want {
					t.Errorf("подпись поста = %q, ожидалось %q", got, want)
				}
				checkConfirm(t, f.byMethod("editMessageText")[0].body, adRestoredMessage, deprecateButton, "delete_-1001234567890_43")
			},
		},
		{
			name: "фото: delete", data: "delete_-1001234567890_43", msg: confirmMsg(photoReply("cap")),
			wantAnswer: adDeprecatedMessage,
			wantEdits:  []string{"editMessageCaption", "editMessageText"},
			check: func(t *testing.T, f *fakeAPI) {
				body := f.byMethod("editMessageCaption")[0].body
				checkTarget(t, "пост", body, testChannelID, 43)
				if got := jsonString(t, body, "caption"); !strings.HasPrefix(got, post.DeprecatedPrefix) {
					t.Errorf("подпись поста = %q, ожидался префикс %q", got, post.DeprecatedPrefix)
				}
				checkConfirm(t, f.byMethod("editMessageText")[0].body, adDeprecatedMessage, restoreButton, "restore_-1001234567890_43")
			},
		},
		{
			// В альбоме «фото + видео» подпись может стоять на видео: подтверждение
			// отвечает на него, а пост в канале — альбом с подписью на первом фото.
			name: "альбом, подпись на видео: delete", data: "delete_-1001234567890_43", msg: confirmMsg(albumVideo(12, "cap")),
			wantAnswer: adDeprecatedMessage,
			wantEdits:  []string{"editMessageCaption", "editMessageText"},
			check: func(t *testing.T, f *fakeAPI) {
				body := f.byMethod("editMessageCaption")[0].body
				checkTarget(t, "пост", body, testChannelID, 43)
				if got, want := jsonString(t, body, "caption"), post.DeprecatedPrefix+"cap"+post.AuthorSuffix+"Иван"; got != want {
					t.Errorf("подпись поста = %q, ожидалось %q", got, want)
				}
			},
		},
		{
			name: "альбом, подпись на документе: restore", data: "restore_-1001234567890_43",
			msg: confirmMsg(&telego.Message{
				MessageID: 13, Date: 1, Caption: "cap", Document: &telego.Document{FileID: "d"},
				From: &telego.User{ID: testAuthorID, FirstName: "Иван"},
			}),
			wantAnswer: adRestoredMessage,
			wantEdits:  []string{"editMessageCaption", "editMessageText"},
		},
		{
			name: "текст: restore", data: "restore_-1001234567890_42", msg: confirmMsg(textReply()),
			wantAnswer: adRestoredMessage,
			wantEdits:  []string{"editMessageText", "editMessageText"},
			check: func(t *testing.T, f *fakeAPI) {
				edits := f.byMethod("editMessageText")
				body := edits[0].body
				checkTarget(t, "пост", body, testChannelID, 42)
				if got := jsonString(t, body, "text"); strings.HasPrefix(got, post.DeprecatedPrefix) {
					t.Errorf("текст восстановленного поста = %q, префикса %q быть не должно", got, post.DeprecatedPrefix)
				}
				checkConfirm(t, edits[1].body, adRestoredMessage, deprecateButton, "delete_-1001234567890_42")
			},
		},
		{
			name: "ошибка правки текста", data: "delete_-1001234567890_42", msg: confirmMsg(textReply()),
			fail:       map[string]string{"editMessageText": "Bad Request: message to edit not found"},
			wantAnswer: channelEditFailedMessage,
			wantEdits:  []string{"editMessageText"},
		},
		{
			name: "ошибка правки подписи", data: "delete_-1001234567890_43", msg: confirmMsg(photoReply("cap")),
			fail:       map[string]string{"editMessageCaption": "Bad Request: message to edit not found"},
			wantAnswer: channelEditFailedMessage,
			wantEdits:  []string{"editMessageCaption"},
		},
		{
			// Повторное нажатие: пост уже в нужном состоянии, Telegram отвечает
			// «message is not modified». Это успех: подтверждение всё равно
			// переключается, автор видит обычный ответ.
			name: "фото: пост уже помечен", data: "delete_-1001234567890_43", msg: confirmMsg(photoReply("cap")),
			fail:       map[string]string{"editMessageCaption": notModified},
			wantAnswer: adDeprecatedMessage,
			wantEdits:  []string{"editMessageCaption", "editMessageText"},
			check: func(t *testing.T, f *fakeAPI) {
				checkConfirm(t, f.byMethod("editMessageText")[0].body, adDeprecatedMessage, restoreButton, "restore_-1001234567890_43")
			},
		},
		{
			// И пост, и подтверждение уже в нужном состоянии: обе правки
			// отвечают «message is not modified».
			name: "текст: всё уже восстановлено", data: "restore_-1001234567890_42", msg: confirmMsg(textReply()),
			fail:       map[string]string{"editMessageText": notModified},
			wantAnswer: adRestoredMessage,
			wantEdits:  []string{"editMessageText", "editMessageText"},
			quiet:      true,
		},
		{
			name: "ошибка ответа на callback", data: "noop", msg: nil,
			fail:       map[string]string{"answerCallbackQuery": "Bad Request: query is too old"},
			wantAnswer: callbackInvalidMessage,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := testutil.CaptureLog(t)
			vb, f := newTestBot(t, tt.fail)
			vb.handleCallbackQuery(&telego.CallbackQuery{ID: "q1", Data: tt.data, Message: tt.msg})

			answers := f.byMethod("answerCallbackQuery")
			if len(answers) != 1 {
				t.Fatalf("answerCallbackQuery вызван %d раз, ожидался ровно один", len(answers))
			}
			if got := jsonString(t, answers[0].body, "callback_query_id"); got != "q1" {
				t.Errorf("callback_query_id = %q, ожидалось %q", got, "q1")
			}
			if got := jsonString(t, answers[0].body, "text"); got != tt.wantAnswer {
				t.Errorf("текст ответа = %q, ожидалось %q", got, tt.wantAnswer)
			}

			calls := f.allCalls()
			if last := calls[len(calls)-1].method; last != "answerCallbackQuery" {
				t.Errorf("последний вызов %q, ожидался answerCallbackQuery", last)
			}
			var edits []string
			for _, c := range calls {
				if c.method != "answerCallbackQuery" {
					edits = append(edits, c.method)
				}
			}
			if strings.Join(edits, ",") != strings.Join(tt.wantEdits, ",") {
				t.Fatalf("методы правки = %v, ожидалось %v", edits, tt.wantEdits)
			}

			if tt.check != nil {
				tt.check(t, f)
			}
			if out := logs.String(); tt.quiet && (strings.Contains(out, "level=WARN") || strings.Contains(out, "level=ERROR")) {
				t.Errorf("получены WARN или ERROR в логе, ожидалось без них:\n%s", out)
			}
		})
	}
}

// TestConfirmNewAdCallbackData: новое подтверждение получает кнопку
// в прежнем формате callback data.
func TestConfirmNewAdCallbackData(t *testing.T) {
	vb, f := newTestBot(t, nil)
	vb.confirmNewAd(testAuthorID, 10, telego.Message{MessageID: 42, Chat: telego.Chat{ID: testChannelID}})

	sent := f.byMethod("sendMessage")
	if len(sent) != 1 {
		t.Fatalf("sendMessage вызван %d раз, ожидался один", len(sent))
	}
	btn := onlyButton(t, sent[0].body)
	if got := jsonString(t, btn, "text"); got != "Не актуально" {
		t.Errorf("текст кнопки = %q, ожидалось %q", got, "Не актуально")
	}
	if got := jsonString(t, btn, "callback_data"); got != "delete_-1001234567890_42" {
		t.Errorf("callback_data = %q, ожидалось %q", got, "delete_-1001234567890_42")
	}
}
