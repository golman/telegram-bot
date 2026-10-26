package main

import tgbotapi "github.com/mymmrac/telego"

type MediaMessage struct {
	fileid            []string
	username          string
	fullname          string
	userid            int64
	caption           string
	entities          []tgbotapi.MessageEntity
	state             MessageState
	originalmessageid int
	// ticksLeft — сколько шагов тикера альбом в состоянии ToDelete ещё хранится.
	ticksLeft int
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
	MessageStateInit MessageState = iota
	MessageStateToBeSent
	// MessageStateToDelete — альбом отправлен (или отклонён) и хранится ещё
	// albumLateWindowTicks шагов тикера, чтобы опоздавшие фото не создали новый альбом.
	MessageStateToDelete
)

// albumLateWindowTicks — сколько шагов тикера (около минуты при шаге 5 секунд)
// отправленный альбом ждёт опоздавшие фото, прежде чем удалиться.
const albumLateWindowTicks = 12

// newMediaMessage создаёт накопитель альбома по первому полученному элементу.
// Фото и подпись сюда не попадают: их добавляет addItem.
func newMediaMessage(msg *tgbotapi.Message) *MediaMessage {
	mm := &MediaMessage{
		fileid:            []string{},
		state:             MessageStateInit,
		originalmessageid: msg.MessageID,
	}
	if msg.From != nil {
		mm.username = msg.From.Username
		mm.fullname = displayName(*msg.From)
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
