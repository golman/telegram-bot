package post

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

func TestBuild(t *testing.T) {
	newSrc := func(media bool) Source {
		return Source{
			Text: "Продам 🚲 велосипед",
			Entities: []telego.MessageEntity{
				{Type: telego.EntityTypeBold, Offset: 10, Length: 9},
			},
			AuthorID:   42,
			AuthorName: "Иван Петров",
			Media:      media,
		}
	}
	ent := func(typ string, offset, length int) telego.MessageEntity {
		return telego.MessageEntity{Type: typ, Offset: offset, Length: length}
	}
	mention := func(offset int) telego.MessageEntity {
		return telego.MessageEntity{Type: telego.EntityTypeTextMention, Offset: offset, Length: 11, User: &telego.User{ID: 42}}
	}
	const (
		bold   = telego.EntityTypeBold
		strike = telego.EntityTypeStrikethrough
	)

	// Ожидаемые entities перечислены целиком и по порядку: так тест ловит
	// лишние, повторные и пропущенные entities, а не только наличие нужных.
	tests := []struct {
		name         string
		media        bool
		deprecated   bool
		noMarkup     bool // у исходного сообщения нет своей разметки
		wantText     string
		wantEntities []telego.MessageEntity
	}{
		{
			name:     "текстовый пост сохраняет разметку автора",
			wantText: "Продам 🚲 велосипед\n\nby: Иван Петров",
			wantEntities: []telego.MessageEntity{
				mention(25), ent(bold, 10, 9),
			},
		},
		{
			name:     "текстовый пост без разметки не становится жирным",
			noMarkup: true,
			wantText: "Продам 🚲 велосипед\n\nby: Иван Петров",
			wantEntities: []telego.MessageEntity{
				mention(25),
			},
		},
		{
			name:     "подпись к фото сохраняет разметку автора",
			media:    true,
			wantText: "Продам 🚲 велосипед\n\nby: Иван Петров",
			wantEntities: []telego.MessageEntity{
				mention(25), ent(bold, 10, 9),
			},
		},
		{
			name:       "неактуальный текстовый пост",
			deprecated: true,
			wantText:   "Не актуально.\n\nПродам 🚲 велосипед\n\nby: Иван Петров",
			wantEntities: []telego.MessageEntity{
				ent(strike, 15, 19),
				ent(strike, 34, 6),
				mention(40), ent(strike, 40, 11),
				ent(bold, 25, 9),
			},
		},
		{
			name:       "неактуальная подпись к фото",
			media:      true,
			deprecated: true,
			wantText:   "Не актуально.\n\nПродам 🚲 велосипед\n\nby: Иван Петров",
			wantEntities: []telego.MessageEntity{
				ent(strike, 15, 19),
				ent(strike, 34, 6),
				mention(40), ent(strike, 40, 11),
				ent(bold, 25, 9),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newSrc(tt.media)
			if tt.noMarkup {
				src.Entities = nil
			}
			text, ents := Build(src, tt.deprecated)

			if text != tt.wantText {
				t.Errorf("текст = %q, ожидалось %q", text, tt.wantText)
			}
			if !reflect.DeepEqual(ents, tt.wantEntities) {
				t.Errorf("entities = %+v\nожидалось  %+v", ents, tt.wantEntities)
			}
			if !tt.noMarkup && src.Entities[0].Offset != 10 {
				t.Errorf("исходные entities изменены: offset = %d, ожидалось 10", src.Entities[0].Offset)
			}
		})
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name string
		user telego.User
		want string
	}{
		{"имя и фамилия", telego.User{FirstName: "Иван", LastName: "Петров"}, "Иван Петров"},
		{"без фамилии", telego.User{FirstName: "Иван"}, "Иван"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DisplayName(tt.user); got != tt.want {
				t.Errorf("DisplayName = %q, ожидалось %q", got, tt.want)
			}
		})
	}
}

func TestFromMessage(t *testing.T) {
	author := &telego.User{ID: 7, FirstName: "A"}
	textEnts := []telego.MessageEntity{{Type: telego.EntityTypeItalic, Offset: 0, Length: 3}}
	capEnts := []telego.MessageEntity{{Type: telego.EntityTypeBold, Offset: 0, Length: 3}}
	photo := []telego.PhotoSize{{FileID: "f1"}}

	tests := []struct {
		name         string
		msg          *telego.Message
		wantOK       bool
		wantMedia    bool
		wantText     string
		wantAuthor   string
		wantEntities []telego.MessageEntity
	}{
		{name: "nil", msg: nil},
		{
			name:         "текст",
			msg:          &telego.Message{Text: "abc", Entities: textEnts, From: author},
			wantOK:       true,
			wantText:     "abc",
			wantAuthor:   "A",
			wantEntities: textEnts,
		},
		{
			name:         "фото с подписью",
			msg:          &telego.Message{Photo: photo, Caption: "cap", CaptionEntities: capEnts, From: author},
			wantOK:       true,
			wantMedia:    true,
			wantText:     "cap",
			wantAuthor:   "A",
			wantEntities: capEnts,
		},
		{
			name: "фото без подписи",
			msg:  &telego.Message{Photo: photo, From: author},
		},
		{
			name: "нет автора",
			msg:  &telego.Message{Text: "abc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, ok := FromMessage(tt.msg)
			if ok != tt.wantOK {
				t.Fatalf("ok = %t, ожидалось %t", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if src.Media != tt.wantMedia {
				t.Errorf("Media = %t, ожидалось %t", src.Media, tt.wantMedia)
			}
			if src.Text != tt.wantText {
				t.Errorf("Text = %q, ожидалось %q", src.Text, tt.wantText)
			}
			if src.AuthorName != tt.wantAuthor {
				t.Errorf("AuthorName = %q, ожидалось %q", src.AuthorName, tt.wantAuthor)
			}
			if src.AuthorID != 7 {
				t.Errorf("AuthorID = %d, ожидалось 7", src.AuthorID)
			}
			if len(src.Entities) != len(tt.wantEntities) {
				t.Fatalf("entities = %+v, ожидалось %+v", src.Entities, tt.wantEntities)
			}
			for i := range tt.wantEntities {
				if src.Entities[i] != tt.wantEntities[i] {
					t.Errorf("entities[%d] = %+v, ожидалось %+v", i, src.Entities[i], tt.wantEntities[i])
				}
			}
		})
	}
}

// TestTextLimit: текст автора длиной ровно TextLimit даёт пост,
// который и после пометки «Не актуально» точно укладывается в лимит Telegram.
func TestTextLimit(t *testing.T) {
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
			src := Source{AuthorID: 42, AuthorName: "Иван 🚲", Media: tt.media}
			n := TextLimit(src)
			// Эмодзи — 2 UTF-16 units, поэтому длина считается не в рунах.
			src.Text = strings.Repeat("🚲", n/2) + strings.Repeat("я", n%2)

			deprecated, _ := Build(src, true)
			if got := tu.UTF16TextLen(deprecated); got != tt.limit {
				t.Errorf("длина поста «Не актуально» = %d, ожидалась ровно %d", got, tt.limit)
			}
			published, _ := Build(src, false)
			if got := tu.UTF16TextLen(published); got > tt.limit {
				t.Errorf("длина опубликованного поста = %d, больше лимита %d", got, tt.limit)
			}
		})
	}
}
