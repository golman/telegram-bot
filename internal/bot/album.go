package bot

import (
	"time"

	tgbotapi "github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/post"
)

type MediaMessage struct {
	fileid            []string
	fullname          string
	userid            int64
	caption           string
	entities          []tgbotapi.MessageEntity
	state             MessageState
	originalmessageid int
	// lastItemAt — когда пришёл последний элемент; от него отсчитывается тишина.
	lastItemAt time.Time
	// expiresAt — когда удалить отправленный или отклонённый альбом (состояние ToDelete).
	expiresAt time.Time
	// lateNotified — автор уже получил AlbumItemLateMessage.
	lateNotified bool
	// skipped — сколько элементов альбома не фото (видео, файлы); они не публикуются.
	skipped int
	// rejected — автор не прошёл проверку подписки или фильтры, элементы отбрасываются.
	rejected bool
	// promised — автору уже отправлено MediaGroupDelayMessage.
	promised bool
}

type MessageState int

const (
	// MessageStateInit — альбом собирается и ждёт albumQuietPeriod тишины.
	MessageStateInit MessageState = iota
	// MessageStateToDelete — альбом отправлен (или отклонён) и хранится до
	// expiresAt, чтобы опоздавшие фото не создали новый альбом.
	MessageStateToDelete
)

const (
	// albumQuietPeriod — сколько ждать после последнего фото альбома, прежде чем
	// отправить его. Telegram присылает фото альбома отдельными обновлениями,
	// при медленной загрузке — с паузами.
	albumQuietPeriod = 3 * time.Second
	// albumLateWindow — сколько отправленный альбом ждёт опоздавшие фото.
	albumLateWindow = time.Minute
	// albumTickInterval — шаг тикера, который проверяет альбомы.
	albumTickInterval = time.Second
)

// newMediaMessage создаёт накопитель альбома по первому полученному элементу.
// Фото и подпись сюда не попадают: их добавляет addItem.
func newMediaMessage(msg *tgbotapi.Message) *MediaMessage {
	mm := &MediaMessage{
		fileid:            []string{},
		state:             MessageStateInit,
		originalmessageid: msg.MessageID,
	}
	if msg.From != nil {
		mm.fullname = post.DisplayName(*msg.From)
		mm.userid = msg.From.ID
	}
	return mm
}

// addItem добавляет элемент альбома: самый большой размер фото и, если у элемента
// есть подпись, её текст, entities и ID сообщения, на которое ответит подтверждение.
// Элемент без фото (видео, файл) только считается в skipped, но его подпись берётся.
// Подпись может прийти на любом элементе, а не только на первом. Если подписи есть
// на нескольких элементах, побеждает последняя (вместе с её entities и ID),
// а подписи остальных элементов не публикуются.
func (mm *MediaMessage) addItem(msg *tgbotapi.Message) {
	if len(msg.Photo) > 0 {
		mm.fileid = append(mm.fileid, msg.Photo[len(msg.Photo)-1].FileID)
	} else {
		mm.skipped++
	}
	if msg.Caption != "" {
		mm.caption = msg.Caption
		mm.entities = msg.CaptionEntities
		mm.originalmessageid = msg.MessageID
	}
}
