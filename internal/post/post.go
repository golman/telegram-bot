// Package post собирает вид объявления в канале: текст, entities и лимиты длины.
package post

import (
	"strings"

	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

// Вид поста в канале (собирает Build).
const (
	// DeprecatedPrefix ставится перед зачёркнутым текстом неактуального объявления.
	DeprecatedPrefix = "Не актуально.\n\n"
	// AuthorSuffix отделяет текст объявления от упоминания автора.
	AuthorSuffix = "\n\nby: "
)

// Source — исходные данные объявления, из которых собирается пост в канале.
type Source struct {
	Text       string
	Entities   []telego.MessageEntity
	AuthorID   int64
	AuthorName string
	Media      bool
}

// Build собирает текст поста и его entities. Используется и при публикации,
// и при пометке «Не актуально» / восстановлении, чтобы пост не менял вид.
// Своего форматирования тело не получает: в пост попадает только разметка автора.
// Пользовательские entities копируются со сдвигом на длину префикса (в UTF-16).
func Build(src Source, deprecated bool) (string, []telego.MessageEntity) {
	body := tu.Entity(src.Text)
	suffix := tu.Entity(AuthorSuffix)
	author := tu.Entity(src.AuthorName).TextMentionWithID(src.AuthorID)

	parts := make([]tu.MessageEntityCollection, 0, 4)
	shift := 0
	if deprecated {
		parts = append(parts, tu.Entity(DeprecatedPrefix))
		body = body.Strikethrough()
		suffix = suffix.Strikethrough()
		author = author.Strikethrough()
		shift = tu.UTF16TextLen(DeprecatedPrefix)
	}
	parts = append(parts, body, suffix, author)

	text, entities := tu.MessageEntities(parts...)
	for _, e := range src.Entities {
		e.Offset += shift
		entities = append(entities, e)
	}
	return text, entities
}

// maxTextLen и maxCaptionLen — лимиты Telegram на длину текста сообщения
// и подписи медиа (в UTF-16).
const (
	maxTextLen    = 4096
	maxCaptionLen = 1024
)

// TextLimit возвращает максимальную длину текста автора (в UTF-16),
// при которой пост укладывается в лимит Telegram. Считается по самому длинному
// виду поста — с пометкой «Не актуально», иначе кнопка не смогла бы его изменить.
func TextLimit(src Source) int {
	limit := maxTextLen
	if src.Media {
		limit = maxCaptionLen
	}
	text, _ := Build(src, true)
	return limit - (tu.UTF16TextLen(text) - tu.UTF16TextLen(src.Text))
}

// DisplayName возвращает имя пользователя для приписки «by:», без висящего пробела.
func DisplayName(u telego.User) string {
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

// FromMessage собирает Source из исходного сообщения автора: текст у текстового
// сообщения, подпись — у любого медиа (фото, а в альбоме и видео или файла,
// если подпись стоит на нём). Возвращает false, если сообщения нет, у него
// нет автора или нет ни текста, ни подписи.
func FromMessage(m *telego.Message) (Source, bool) {
	if m == nil || m.From == nil {
		return Source{}, false
	}
	src := Source{
		AuthorID:   m.From.ID,
		AuthorName: DisplayName(*m.From),
	}
	if m.Text != "" {
		src.Text = m.Text
		src.Entities = m.Entities
	} else {
		src.Text = m.Caption
		src.Entities = m.CaptionEntities
		src.Media = true
	}
	if src.Text == "" {
		return Source{}, false
	}
	return src, true
}
