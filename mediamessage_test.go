package main

import (
	"reflect"
	"testing"

	tgbotapi "github.com/mymmrac/telego"
)

// TestMediaMessageAddItemCaption проверяет, что подпись альбома, её entities
// и ID сообщения для подтверждения берутся с того элемента, где есть подпись.
func TestMediaMessageAddItemCaption(t *testing.T) {
	author := &tgbotapi.User{ID: 42, FirstName: "Иван"}
	// photo возвращает размеры одного фото по возрастанию, как их присылает Telegram.
	photo := func(prefix string) []tgbotapi.PhotoSize {
		return []tgbotapi.PhotoSize{
			{FileID: prefix + "-small", Width: 90, Height: 90},
			{FileID: prefix + "-big", Width: 1280, Height: 1280},
		}
	}
	boldEntities := []tgbotapi.MessageEntity{
		{Type: tgbotapi.EntityTypeBold, Offset: 0, Length: 6},
	}
	italicEntities := []tgbotapi.MessageEntity{
		{Type: tgbotapi.EntityTypeItalic, Offset: 0, Length: 5},
	}

	tests := []struct {
		name          string
		caption1      string
		entities1     []tgbotapi.MessageEntity
		caption2      string
		entities2     []tgbotapi.MessageEntity
		wantCaption   string
		wantEntities  []tgbotapi.MessageEntity
		wantOrigMsgID int
	}{
		{
			name:          "подпись на втором элементе",
			caption2:      "Продам",
			entities2:     boldEntities,
			wantCaption:   "Продам",
			wantEntities:  boldEntities,
			wantOrigMsgID: 101,
		},
		{
			name:          "подпись на первом элементе, второй без подписи",
			caption1:      "Продам",
			entities1:     boldEntities,
			wantCaption:   "Продам",
			wantEntities:  boldEntities,
			wantOrigMsgID: 100,
		},
		{
			name:          "подпись на обоих элементах, побеждает последняя",
			caption1:      "Продам",
			entities1:     boldEntities,
			caption2:      "Куплю",
			entities2:     italicEntities,
			wantCaption:   "Куплю",
			wantEntities:  italicEntities,
			wantOrigMsgID: 101,
		},
		{
			name:          "подписи нет ни на одном элементе",
			wantCaption:   "",
			wantEntities:  nil,
			wantOrigMsgID: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m1 := &tgbotapi.Message{
				MessageID:       100,
				From:            author,
				MediaGroupID:    "g1",
				Photo:           photo("p1"),
				Caption:         tt.caption1,
				CaptionEntities: tt.entities1,
			}
			m2 := &tgbotapi.Message{
				MessageID:       101,
				From:            author,
				MediaGroupID:    "g1",
				Photo:           photo("p2"),
				Caption:         tt.caption2,
				CaptionEntities: tt.entities2,
			}

			mm := newMediaMessage(m1)
			mm.addItem(m1)
			mm.addItem(m2)

			if mm.caption != tt.wantCaption {
				t.Errorf("caption = %q, want %q", mm.caption, tt.wantCaption)
			}
			if !reflect.DeepEqual(mm.entities, tt.wantEntities) {
				t.Errorf("entities = %+v, want %+v", mm.entities, tt.wantEntities)
			}
			if mm.originalmessageid != tt.wantOrigMsgID {
				t.Errorf("originalmessageid = %d, want %d", mm.originalmessageid, tt.wantOrigMsgID)
			}
			if want := []string{"p1-big", "p2-big"}; !reflect.DeepEqual(mm.fileid, want) {
				t.Errorf("fileid = %v, want %v", mm.fileid, want)
			}
			if mm.state != MessageStateInit {
				t.Errorf("state = %v, want %v", mm.state, MessageStateInit)
			}
			if mm.userid != 42 || mm.fullname != "Иван" {
				t.Errorf("автор = (%d, %q), want (42, %q)", mm.userid, mm.fullname, "Иван")
			}
		})
	}
}
