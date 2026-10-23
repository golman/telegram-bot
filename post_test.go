package main

import (
	"reflect"
	"strings"
	"testing"

	tgbotapi "github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
)

func TestBuildPost(t *testing.T) {
	newSrc := func(media bool) postSource {
		return postSource{
			text: "Продам 🚲 велосипед",
			entities: []tgbotapi.MessageEntity{
				{Type: tgbotapi.EntityTypeBold, Offset: 10, Length: 9},
			},
			authorID:   42,
			authorName: "Иван Петров",
			media:      media,
		}
	}
	ent := func(typ string, offset, length int) tgbotapi.MessageEntity {
		return tgbotapi.MessageEntity{Type: typ, Offset: offset, Length: length}
	}
	mention := func(offset int) tgbotapi.MessageEntity {
		return tgbotapi.MessageEntity{Type: tgbotapi.EntityTypeTextMention, Offset: offset, Length: 11, User: &tgbotapi.User{ID: 42}}
	}
	const (
		bold   = tgbotapi.EntityTypeBold
		strike = tgbotapi.EntityTypeStrikethrough
	)

	// Ожидаемые entities перечислены целиком и по порядку: так тест ловит
	// лишние, повторные и пропущенные entities, а не только наличие нужных.
	tests := []struct {
		name         string
		media        bool
		deprecated   bool
		noMarkup     bool // у исходного сообщения нет своей разметки
		wantText     string
		wantEntities []tgbotapi.MessageEntity
	}{
		{
			name:     "текстовый пост сохраняет разметку автора",
			wantText: "Продам 🚲 велосипед\n\n by: Иван Петров",
			wantEntities: []tgbotapi.MessageEntity{
				mention(26), ent(bold, 10, 9),
			},
		},
		{
			name:     "текстовый пост без разметки не становится жирным",
			noMarkup: true,
			wantText: "Продам 🚲 велосипед\n\n by: Иван Петров",
			wantEntities: []tgbotapi.MessageEntity{
				mention(26),
			},
		},
		{
			name:     "подпись к фото сохраняет разметку автора",
			media:    true,
			wantText: "Продам 🚲 велосипед\n\n by: Иван Петров",
			wantEntities: []tgbotapi.MessageEntity{
				mention(26), ent(bold, 10, 9),
			},
		},
		{
			name:       "неактуальный текстовый пост",
			deprecated: true,
			wantText:   "Не актуально.\n\nПродам 🚲 велосипед\n\n by: Иван Петров",
			wantEntities: []tgbotapi.MessageEntity{
				ent(strike, 15, 19),
				ent(strike, 34, 7),
				mention(41), ent(strike, 41, 11),
				ent(bold, 25, 9),
			},
		},
		{
			name:       "неактуальная подпись к фото",
			media:      true,
			deprecated: true,
			wantText:   "Не актуально.\n\nПродам 🚲 велосипед\n\n by: Иван Петров",
			wantEntities: []tgbotapi.MessageEntity{
				ent(strike, 15, 19),
				ent(strike, 34, 7),
				mention(41), ent(strike, 41, 11),
				ent(bold, 25, 9),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newSrc(tt.media)
			if tt.noMarkup {
				src.entities = nil
			}
			text, ents := buildPost(src, tt.deprecated)

			if text != tt.wantText {
				t.Errorf("текст = %q, ожидалось %q", text, tt.wantText)
			}
			if !reflect.DeepEqual(ents, tt.wantEntities) {
				t.Errorf("entities = %+v\nожидалось  %+v", ents, tt.wantEntities)
			}
			if !tt.noMarkup && src.entities[0].Offset != 10 {
				t.Errorf("исходные entities изменены: offset = %d, ожидалось 10", src.entities[0].Offset)
			}
		})
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name string
		user tgbotapi.User
		want string
	}{
		{"имя и фамилия", tgbotapi.User{FirstName: "Иван", LastName: "Петров"}, "Иван Петров"},
		{"без фамилии", tgbotapi.User{FirstName: "Иван"}, "Иван"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := displayName(tt.user); got != tt.want {
				t.Errorf("displayName = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

func TestPostSourceFromMessage(t *testing.T) {
	author := &tgbotapi.User{ID: 7, FirstName: "A"}
	textEnts := []tgbotapi.MessageEntity{{Type: tgbotapi.EntityTypeItalic, Offset: 0, Length: 3}}
	capEnts := []tgbotapi.MessageEntity{{Type: tgbotapi.EntityTypeBold, Offset: 0, Length: 3}}
	photo := []tgbotapi.PhotoSize{{FileID: "f1"}}

	tests := []struct {
		name         string
		msg          *tgbotapi.Message
		wantOK       bool
		wantMedia    bool
		wantText     string
		wantAuthor   string
		wantEntities []tgbotapi.MessageEntity
	}{
		{name: "nil", msg: nil},
		{
			name:         "текст",
			msg:          &tgbotapi.Message{Text: "abc", Entities: textEnts, From: author},
			wantOK:       true,
			wantText:     "abc",
			wantAuthor:   "A",
			wantEntities: textEnts,
		},
		{
			name:         "фото с подписью",
			msg:          &tgbotapi.Message{Photo: photo, Caption: "cap", CaptionEntities: capEnts, From: author},
			wantOK:       true,
			wantMedia:    true,
			wantText:     "cap",
			wantAuthor:   "A",
			wantEntities: capEnts,
		},
		{
			name: "фото без подписи",
			msg:  &tgbotapi.Message{Photo: photo, From: author},
		},
		{
			name: "нет автора",
			msg:  &tgbotapi.Message{Text: "abc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, ok := postSourceFromMessage(tt.msg)
			if ok != tt.wantOK {
				t.Fatalf("ok = %t, ожидалось %t", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if src.media != tt.wantMedia {
				t.Errorf("media = %t, ожидалось %t", src.media, tt.wantMedia)
			}
			if src.text != tt.wantText {
				t.Errorf("text = %q, ожидалось %q", src.text, tt.wantText)
			}
			if src.authorName != tt.wantAuthor {
				t.Errorf("authorName = %q, ожидалось %q", src.authorName, tt.wantAuthor)
			}
			if src.authorID != 7 {
				t.Errorf("authorID = %d, ожидалось 7", src.authorID)
			}
			if len(src.entities) != len(tt.wantEntities) {
				t.Fatalf("entities = %+v, ожидалось %+v", src.entities, tt.wantEntities)
			}
			for i := range tt.wantEntities {
				if src.entities[i] != tt.wantEntities[i] {
					t.Errorf("entities[%d] = %+v, ожидалось %+v", i, src.entities[i], tt.wantEntities[i])
				}
			}
		})
	}
}

// TestPostTextLimit: текст автора длиной ровно postTextLimit даёт пост,
// который и после пометки «Не актуально» точно укладывается в лимит Telegram.
func TestPostTextLimit(t *testing.T) {
	tests := []struct {
		name  string
		media bool
		limit int
	}{
		{"текст", false, maxTextLen},
		{"подпись медиа", true, maxCaptionLen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := postSource{authorID: 42, authorName: "Иван 🚲", media: tt.media}
			n := postTextLimit(src)
			// Эмодзи — 2 UTF-16 units, поэтому длина считается не в рунах.
			src.text = strings.Repeat("🚲", n/2) + strings.Repeat("я", n%2)

			deprecated, _ := buildPost(src, true)
			if got := telegoutil.UTF16TextLen(deprecated); got != tt.limit {
				t.Errorf("длина поста «Не актуально» = %d, ожидалась ровно %d", got, tt.limit)
			}
			published, _ := buildPost(src, false)
			if got := telegoutil.UTF16TextLen(published); got > tt.limit {
				t.Errorf("длина опубликованного поста = %d, больше лимита %d", got, tt.limit)
			}
		})
	}
}
