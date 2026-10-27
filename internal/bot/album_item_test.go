package bot

import (
	"reflect"
	"testing"

	"github.com/mymmrac/telego"
)

// TestAlbumAddItemCaption проверяет, что подпись альбома, её entities
// и ID сообщения для подтверждения берутся с того элемента, где есть подпись.
func TestAlbumAddItemCaption(t *testing.T) {
	author := &telego.User{ID: testAuthorID, FirstName: "Иван"}
	// photo возвращает размеры одного фото по возрастанию, как их присылает Telegram.
	photo := func(prefix string) []telego.PhotoSize {
		return []telego.PhotoSize{
			{FileID: prefix + "-small", Width: 90, Height: 90},
			{FileID: prefix + "-big", Width: 1280, Height: 1280},
		}
	}
	boldEntities := []telego.MessageEntity{
		{Type: telego.EntityTypeBold, Offset: 0, Length: 6},
	}
	italicEntities := []telego.MessageEntity{
		{Type: telego.EntityTypeItalic, Offset: 0, Length: 5},
	}

	tests := []struct {
		name          string
		caption1      string
		entities1     []telego.MessageEntity
		caption2      string
		entities2     []telego.MessageEntity
		wantCaption   string
		wantEntities  []telego.MessageEntity
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
			m1 := &telego.Message{
				MessageID:       100,
				From:            author,
				MediaGroupID:    "g1",
				Photo:           photo("p1"),
				Caption:         tt.caption1,
				CaptionEntities: tt.entities1,
			}
			m2 := &telego.Message{
				MessageID:       101,
				From:            author,
				MediaGroupID:    "g1",
				Photo:           photo("p2"),
				Caption:         tt.caption2,
				CaptionEntities: tt.entities2,
			}

			a := newAlbum(m1)
			a.addItem(m1)
			a.addItem(m2)

			if a.caption != tt.wantCaption {
				t.Errorf("подпись = %q, ожидалось %q", a.caption, tt.wantCaption)
			}
			if !reflect.DeepEqual(a.entities, tt.wantEntities) {
				t.Errorf("entities = %+v, ожидалось %+v", a.entities, tt.wantEntities)
			}
			if a.replyToID != tt.wantOrigMsgID {
				t.Errorf("replyToID = %d, ожидалось %d", a.replyToID, tt.wantOrigMsgID)
			}
			if want := []string{"p1-big", "p2-big"}; !reflect.DeepEqual(a.fileIDs, want) {
				t.Errorf("fileIDs = %v, ожидалось %v", a.fileIDs, want)
			}
			if a.state != albumCollecting {
				t.Errorf("состояние = %v, ожидалось %v", a.state, albumCollecting)
			}
			if a.authorID != testAuthorID || a.authorName != "Иван" {
				t.Errorf("автор = (%d, %q), ожидалось (%d, %q)", a.authorID, a.authorName, testAuthorID, "Иван")
			}
		})
	}
}
