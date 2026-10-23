package main

import (
	"strings"

	tgbotapi "github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoutil"
)

// postSource — исходные данные объявления, из которых собирается пост в канале.
type postSource struct {
	text       string
	entities   []tgbotapi.MessageEntity
	authorID   int64
	authorName string
	media      bool
}

// buildPost собирает текст поста и его entities. Используется и при публикации,
// и при пометке «Не актуально» / восстановлении, чтобы пост не менял вид.
// Своего форматирования тело не получает: в пост попадает только разметка автора.
// Пользовательские entities копируются со сдвигом на длину префикса (в UTF-16).
func buildPost(src postSource, deprecated bool) (string, []tgbotapi.MessageEntity) {
	body := telegoutil.Entity(src.text)
	suffix := telegoutil.Entity(AuthorSuffix)
	author := telegoutil.Entity(src.authorName).TextMentionWithID(src.authorID)

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
	for _, e := range src.entities {
		e.Offset += shift
		entities = append(entities, e)
	}
	return text, entities
}

// displayName возвращает имя пользователя для приписки «by:», без висящего пробела.
func displayName(u tgbotapi.User) string {
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

// postSourceFromMessage собирает postSource из исходного сообщения автора.
// Возвращает false, если сообщения нет, у него нет автора или нет текста/подписи.
func postSourceFromMessage(m *tgbotapi.Message) (postSource, bool) {
	if m == nil || m.From == nil {
		return postSource{}, false
	}
	src := postSource{
		authorID:   m.From.ID,
		authorName: displayName(*m.From),
	}
	if m.Photo != nil {
		src.text = m.Caption
		src.entities = m.CaptionEntities
		src.media = true
	} else {
		src.text = m.Text
		src.entities = m.Entities
	}
	if src.text == "" {
		return postSource{}, false
	}
	return src, true
}
