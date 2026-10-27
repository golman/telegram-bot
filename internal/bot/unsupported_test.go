package bot

import (
	"reflect"
	"testing"

	"github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/post"
	"github.com/golman/telegram-bot/internal/testutil"
)

// TestUnsupportedMessage: всё, кроме текста и фото, отклоняется одним
// понятным сообщением, а в канал ничего не уходит.
func TestUnsupportedMessage(t *testing.T) {
	tests := []struct {
		name string
		set  func(m *telego.Message)
	}{
		{"стикер", func(m *telego.Message) { m.Sticker = &telego.Sticker{FileID: "s"} }},
		{"голосовое", func(m *telego.Message) { m.Voice = &telego.Voice{FileID: "v"} }},
		{"видео с подписью", func(m *telego.Message) { m.Video = &telego.Video{FileID: "v"}; m.Caption = "Продам" }},
		{"документ с подписью", func(m *telego.Message) { m.Document = &telego.Document{FileID: "d"}; m.Caption = "Продам" }},
		{"геолокация", func(m *telego.Message) { m.Location = &telego.Location{Latitude: 1, Longitude: 2} }},
		{"контакт", func(m *telego.Message) { m.Contact = &telego.Contact{PhoneNumber: "+1", FirstName: "Иван"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.CaptureLog(t)
			b, f := newTestBot(t, nil)
			b.albums = map[string]*album{}
			m := authorMessage(100)
			tt.set(m)

			b.handleUpdate(telego.Update{Message: m})

			if got, want := authorTexts(t, f), []string{unsupportedMessage}; !reflect.DeepEqual(got, want) {
				t.Errorf("автору ушли %q, ожидалось %q", got, want)
			}
			if n := channelCalls(t, f, "sendMessage") + channelCalls(t, f, "sendMediaGroup"); n != 0 {
				t.Errorf("в канал ушло %d запросов, ожидалось 0", n)
			}
		})
	}
}

// TestSupportedMessagesStillPublished: текст и одиночное фото публикуются как раньше.
func TestSupportedMessagesStillPublished(t *testing.T) {
	tests := []struct {
		name   string
		set    func(m *telego.Message)
		method string
	}{
		{"текст", func(m *telego.Message) { m.Text = "Продам" }, "sendMessage"},
		{"фото", func(m *telego.Message) { m.Photo = []telego.PhotoSize{{FileID: "p"}}; m.Caption = "Продам" }, "sendMediaGroup"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.CaptureLog(t)
			b, f := newTestBot(t, nil)
			b.albums = map[string]*album{}
			m := authorMessage(100)
			tt.set(m)

			b.handleUpdate(telego.Update{Message: m})

			if n := channelCalls(t, f, tt.method); n != 1 {
				t.Errorf("%s в канал вызван %d раз, ожидался один", tt.method, n)
			}
			if got, want := authorTexts(t, f), []string{adPublishedMessage}; !reflect.DeepEqual(got, want) {
				t.Errorf("автору ушли %q, ожидалось %q", got, want)
			}
		})
	}
}

// TestAlbumWithUnsupportedItems: видео в альбоме не публикуются, но подпись
// с видео не теряется; автор один раз узнаёт о пропущенных видео. Альбом
// из одних видео не публикуется, автор получает один отказ.
func TestAlbumWithUnsupportedItems(t *testing.T) {
	tests := []struct {
		name      string
		items     []*telego.Message
		wantMedia []string // file ID опубликованных фото; nil — альбом не публикуется
		wantTexts []string
	}{
		{
			name:      "фото и видео, подпись на видео",
			items:     []*telego.Message{albumItem(100, "p1", "", nil), albumVideo(101, "Продам"), albumVideo(102, "")},
			wantMedia: []string{"p1-big"},
			wantTexts: []string{mediaGroupDelayMessage, adPublishedMessage, albumSkippedMessage},
		},
		{
			name:  "только видео",
			items: []*telego.Message{albumVideo(100, "Продам"), albumVideo(101, "")},
			// Без фото публикация не обещается: сразу один отказ.
			wantTexts: []string{unsupportedMessage},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.CaptureLog(t)
			b, f := newTestBot(t, nil)
			b.albums = map[string]*album{}
			clock := useFakeClock(b)
			for _, m := range tt.items {
				b.handleUpdate(telego.Update{Message: m})
			}
			clock.wait(b, albumQuietPeriod)

			groups := f.byMethod("sendMediaGroup")
			if tt.wantMedia == nil {
				if len(groups) != 0 {
					t.Errorf("sendMediaGroup вызван %d раз, альбом публиковаться не должен", len(groups))
				}
			} else {
				if len(groups) != 1 {
					t.Fatalf("sendMediaGroup вызван %d раз, ожидался один", len(groups))
				}
				var got []string
				media := jsonField[[]any](t, groups[0].body, "media")
				for i := range media {
					got = append(got, jsonString(t, jsonIndex[map[string]any](t, media, i), "media"))
				}
				if !reflect.DeepEqual(got, tt.wantMedia) {
					t.Errorf("опубликованы %v, ожидались %v", got, tt.wantMedia)
				}
				first := jsonIndex[map[string]any](t, media, 0)
				if want := "Продам" + post.AuthorSuffix + "Иван"; jsonString(t, first, "caption") != want {
					t.Errorf("подпись %q, ожидалась %q", jsonString(t, first, "caption"), want)
				}
			}
			if got := authorTexts(t, f); !reflect.DeepEqual(got, tt.wantTexts) {
				t.Errorf("автору ушли %q, ожидалось %q", got, tt.wantTexts)
			}
		})
	}
}

// TestHandleUpdateOnlyPrivateChats: бот отвечает только в личном чате,
// определяя его по типу чата, а не по знаку ID. Сообщение без автора
// отбрасывается без паники и без запросов к API.
func TestHandleUpdateOnlyPrivateChats(t *testing.T) {
	tests := []struct {
		name     string
		set      func(m *telego.Message)
		wantSent bool
	}{
		{"личный чат", func(m *telego.Message) {}, true},
		{"группа", func(m *telego.Message) { m.Chat = telego.Chat{ID: -100, Type: telego.ChatTypeGroup} }, false},
		{"супергруппа", func(m *telego.Message) { m.Chat = telego.Chat{ID: -1001, Type: telego.ChatTypeSupergroup} }, false},
		// Положительный ID ещё не значит личный чат.
		{"группа с положительным ID", func(m *telego.Message) { m.Chat.Type = telego.ChatTypeGroup }, false},
		{"без автора", func(m *telego.Message) { m.From = nil }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.CaptureLog(t)
			b, f := newTestBot(t, nil)
			b.albums = map[string]*album{}
			// Проверка подписки включена: без автора она разыменовала бы nil.
			b.authEnabled = true
			f.respond("getChatMember", `{"status":"member","user":{"id":42,"is_bot":false,"first_name":"Иван"}}`)
			m := authorMessage(100)
			m.Text = "Продам"
			tt.set(m)

			b.handleUpdate(telego.Update{Message: m})

			if got := channelCalls(t, f, "sendMessage") == 1; got != tt.wantSent {
				t.Errorf("публикация в канал: %v, ожидалось %v", got, tt.wantSent)
			}
			if !tt.wantSent && len(f.allCalls()) != 0 {
				t.Errorf("запросов к API %d, ожидалось 0: %v", len(f.allCalls()), f.allCalls())
			}
		})
	}
}
