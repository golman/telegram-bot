// Package post собирает вид объявления в канале: текст, entities и лимиты длины.
package post

import (
	"strings"

	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
)

// Вид поста в канале (собирает Build).
const (
	// DeprecatedPrefix ставится перед зачёркнутым текстом неактуального объявления.
	DeprecatedPrefix = "Не актуально.\n\n"
	// AuthorSuffix отделяет текст объявления от упоминания автора.
	AuthorSuffix = "\n\n by: "
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
	body := telegoutil.Entity(src.Text)
	suffix := telegoutil.Entity(AuthorSuffix)
	author := telegoutil.Entity(src.AuthorName).TextMentionWithID(src.AuthorID)

	parts := make([]telegoutil.MessageEntityCollection, 0, 4)
	shift := 0
	if deprecated {
		parts = append(parts, telegoutil.Entity(DeprecatedPrefix))
		body = body.Strikethrough()
		suffix = suffix.Strikethrough()
		author = author.Strikethrough()
		shift = telegoutil.UTF16TextLen(DeprecatedPrefix)
	}
	parts = append(parts, body, suffix, author)

	text, entities := telegoutil.MessageEntities(parts...)
	for _, e := range src.Entities {
		e.Offset += shift
		entities = append(entities, e)
	}
	return text, entities
}

// MaxTextLen и MaxCaptionLen — лимиты Telegram на длину текста сообщения
// и подписи медиа (в UTF-16).
const (
	MaxTextLen    = 4096
	MaxCaptionLen = 1024
)

// TextLimit возвращает максимальную длину текста автора (в UTF-16),
// при которой пост укладывается в лимит Telegram. Считается по самому длинному
// виду поста — с пометкой «Не актуально», иначе кнопка не смогла бы его изменить.
func TextLimit(src Source) int {
	limit := MaxTextLen
	if src.Media {
		limit = MaxCaptionLen
	}
	text, _ := Build(src, true)
	return limit - (telegoutil.UTF16TextLen(text) - telegoutil.UTF16TextLen(src.Text))
}

// DisplayName возвращает имя пользователя для приписки «by:», без висящего пробела.
func DisplayName(u telego.User) string {
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

// FromMessage собирает Source из исходного сообщения автора.
// Возвращает false, если сообщения нет, у него нет автора или нет текста/подписи.
func FromMessage(m *telego.Message) (Source, bool) {
	if m == nil || m.From == nil {
		return Source{}, false
	}
	src := Source{
		AuthorID:   m.From.ID,
		AuthorName: DisplayName(*m.From),
	}
	if m.Photo != nil {
		src.Text = m.Caption
		src.Entities = m.CaptionEntities
		src.Media = true
	} else {
		src.Text = m.Text
		src.Entities = m.Entities
	}
	if src.Text == "" {
		return Source{}, false
	}
	return src, true
}
