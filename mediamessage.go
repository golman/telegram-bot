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
}

type MessageState int

const (
	MessageStateInit MessageState = iota
	MessageStateToBeSent
	MessageStateToDelete
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
		mm.username = msg.From.Username
		mm.fullname = displayName(*msg.From)
		mm.userid = msg.From.ID
	}
	return mm
}

// addItem добавляет элемент альбома: самый большой размер фото и, если у элемента
// есть подпись, её текст, entities и ID сообщения, на которое ответит подтверждение.
// Подпись может прийти на любом элементе, а не только на первом. Если подписи есть
// на нескольких элементах, побеждает последняя (вместе с её entities и ID),
// а подписи остальных элементов не публикуются.
func (mm *MediaMessage) addItem(msg *tgbotapi.Message) {
	if len(msg.Photo) > 0 {
		mm.fileid = append(mm.fileid, msg.Photo[len(msg.Photo)-1].FileID)
	}
	if msg.Caption != "" {
		mm.caption = msg.Caption
		mm.entities = msg.CaptionEntities
		mm.originalmessageid = msg.MessageID
	}
}
