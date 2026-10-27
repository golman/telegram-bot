package bot

import (
	"fmt"
	"reflect"
	"time"

	"github.com/mymmrac/telego"
)

// fakeClock — управляемые часы для тикера альбомов: время двигает тест.
type fakeClock struct{ t time.Time }

// useFakeClock подставляет боту управляемые часы.
func useFakeClock(b *Bot) *fakeClock {
	c := &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	b.now = func() time.Time { return c.t }
	return c
}

// wait сдвигает время на d и делает шаг тикера.
func (c *fakeClock) wait(b *Bot, d time.Duration) {
	c.t = c.t.Add(d)
	b.tick()
}

// Общие данные тестов. testChannelID и testMediaGroupMessageID — в fakeapi_test.go.
const (
	// testAuthorID — автор объявлений; ID его личного чата с ботом такой же.
	testAuthorID int64 = 42
	// testConfirmMessageID — сообщение-подтверждение с кнопкой в чате автора.
	// Нетипизированная: используется и как int (MessageID), и как int64 (JSON).
	testConfirmMessageID = 500
)

// authorMessage — сообщение автора в личном чате с ботом.
func authorMessage(id int) *telego.Message {
	return &telego.Message{
		MessageID: id,
		Date:      1,
		Chat:      telego.Chat{ID: testAuthorID, Type: "private"},
		From:      &telego.User{ID: testAuthorID, FirstName: "Иван"},
	}
}

// albumItem — элемент альбома автора: два размера фото и, возможно, подпись.
func albumItem(id int, photo, caption string, ents []telego.MessageEntity) *telego.Message {
	return &telego.Message{
		MessageID:       id,
		Date:            1,
		Chat:            telego.Chat{ID: testAuthorID, Type: "private"},
		From:            &telego.User{ID: testAuthorID, FirstName: "Иван"},
		MediaGroupID:    "album",
		Photo:           []telego.PhotoSize{{FileID: photo + "-small"}, {FileID: photo + "-big"}},
		Caption:         caption,
		CaptionEntities: ents,
	}
}

// albumVideo — видео из того же альбома, что albumItem.
func albumVideo(id int, caption string) *telego.Message {
	m := albumItem(id, "", caption, nil)
	m.Photo = nil
	m.Video = &telego.Video{FileID: "v"}
	return m
}

// confirmMsg — подтверждение публикации, отвечающее на исходное сообщение reply.
func confirmMsg(reply *telego.Message) *telego.Message {
	return &telego.Message{
		MessageID:      testConfirmMessageID,
		Date:           1,
		Chat:           telego.Chat{ID: testAuthorID, Type: "private"},
		ReplyToMessage: reply,
	}
}

// textReply — исходное текстовое объявление автора.
func textReply() *telego.Message {
	return &telego.Message{
		MessageID: 10,
		Date:      1,
		Text:      "Продам 🚲",
		From:      &telego.User{ID: testAuthorID, FirstName: "Иван"},
		Entities:  []telego.MessageEntity{{Type: "bold", Offset: 0, Length: 6}},
	}
}

// photoReply — исходное объявление с фото и подписью caption.
func photoReply(caption string) *telego.Message {
	return &telego.Message{
		MessageID: 11,
		Date:      1,
		Caption:   caption,
		Photo:     []telego.PhotoSize{{FileID: "f"}},
		From:      &telego.User{ID: testAuthorID, FirstName: "Иван"},
	}
}

// pendingAlbum — накопленный альбом с подписью в состоянии state.
func pendingAlbum(state albumState) *album {
	return &album{
		fileIDs:    []string{"p1"},
		authorName: "Иван",
		authorID:   testAuthorID,
		caption:    "Продам",
		state:      state,
	}
}

// chatMemberJSON — ответ getChatMember для автора testAuthorID со статусом status.
func chatMemberJSON(status string) string {
	return fmt.Sprintf(`{"status":%q,"user":{"id":%d,"is_bot":false,"first_name":"Иван"}}`, status, testAuthorID)
}

// containsEntity сообщает, есть ли e в списке ents.
func containsEntity(ents []telego.MessageEntity, e telego.MessageEntity) bool {
	for _, got := range ents {
		if reflect.DeepEqual(got, e) {
			return true
		}
	}
	return false
}
