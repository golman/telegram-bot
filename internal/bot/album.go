package bot

import (
	"time"

	"github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/post"
)

type album struct {
	fileIDs    []string
	authorName string
	authorID   int64
	caption    string
	entities   []telego.MessageEntity
	state      albumState
	replyToID  int
	// lastItemAt — когда пришёл последний элемент; от него отсчитывается тишина.
	lastItemAt time.Time
	// expiresAt — когда удалить отправленный или отклонённый альбом (состояние albumDone).
	expiresAt time.Time
	// lateNotified — автор уже получил albumItemLateMessage.
	lateNotified bool
	// published — альбом вышел в канал. Опоздавшие фото неопубликованного
	// альбома (отказ, ошибка API) не порождают albumItemLateMessage: автор
	// уже получил отказ, а публикации не было.
	published bool
	// skipped — сколько элементов альбома не фото (видео, файлы); они не публикуются.
	skipped int
	// rejected — автор не прошёл проверку подписки или фильтры, элементы отбрасываются.
	rejected bool
	// promised — автору уже отправлено mediaGroupDelayMessage.
	promised bool
}

type albumState int

const (
	// albumCollecting — альбом собирается и ждёт albumQuietPeriod тишины.
	albumCollecting albumState = iota
	// albumDone — альбом отправлен (или отклонён) и хранится до
	// expiresAt, чтобы опоздавшие фото не создали новый альбом.
	albumDone
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

// newAlbum создаёт накопитель альбома по первому полученному элементу.
// Фото и подпись сюда не попадают: их добавляет addItem.
func newAlbum(msg *telego.Message) *album {
	return &album{
		fileIDs:    []string{},
		state:      albumCollecting,
		replyToID:  msg.MessageID,
		authorName: post.DisplayName(*msg.From),
		authorID:   msg.From.ID,
	}
}

// source — данные поста из подписи альбома.
func (a *album) source() post.Source {
	return post.Source{
		Text:       a.caption,
		Entities:   a.entities,
		AuthorID:   a.authorID,
		AuthorName: a.authorName,
		Media:      true,
	}
}

// addItem добавляет элемент альбома: самый большой размер фото и, если у элемента
// есть подпись, её текст, entities и ID сообщения, на которое ответит подтверждение.
// Элемент без фото (видео, файл) только считается в skipped, но его подпись берётся.
// Подпись может прийти на любом элементе, а не только на первом. Если подписи есть
// на нескольких элементах, побеждает последняя (вместе с её entities и ID),
// а подписи остальных элементов не публикуются.
func (a *album) addItem(msg *telego.Message) {
	if len(msg.Photo) > 0 {
		a.fileIDs = append(a.fileIDs, msg.Photo[len(msg.Photo)-1].FileID)
	} else {
		a.skipped++
	}
	if msg.Caption != "" {
		a.caption = msg.Caption
		a.entities = msg.CaptionEntities
		a.replyToID = msg.MessageID
	}
}
