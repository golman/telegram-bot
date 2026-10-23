package main

import (
	"context"
	"log"
	"net/http"
	"path"
	"time"

	ta "github.com/mymmrac/telego/telegoapi"
)

// rateLimitCaller повторяет запрос к Bot API, если Telegram ответил
// 429 Too Many Requests, выждав retry_after. Другие ошибки не повторяются:
// после сетевой ошибки запрос мог дойти до Telegram, и повтор опубликовал бы
// объявление дважды. 429 означает, что запрос отклонён, и повтор безопасен.
//
// Ожидание блокирует цикл событий, поэтому оно ограничено maxWait.
type rateLimitCaller struct {
	next ta.Caller
	// attempts — сколько всего попыток, включая первую.
	attempts int
	// maxWait — если retry_after больше, ошибка возвращается без ожидания.
	maxWait time.Duration
	sleep   func(ctx context.Context, d time.Duration) error
}

func newRateLimitCaller(next ta.Caller) *rateLimitCaller {
	return &rateLimitCaller{next: next, attempts: 3, maxWait: 30 * time.Second, sleep: sleepContext}
}

func (c *rateLimitCaller) Call(ctx context.Context, url string, data *ta.RequestData) (*ta.Response, error) {
	for attempt := 1; ; attempt++ {
		resp, err := c.next.Call(ctx, url, data)
		if err != nil || resp.Error == nil || resp.ErrorCode != http.StatusTooManyRequests || attempt >= c.attempts {
			return resp, err
		}
		// Поток уже прочитан первым запросом, повторить его нельзя.
		if data.BodyRaw == nil && data.BodyStream != nil {
			return resp, nil
		}
		wait := time.Second
		if resp.Parameters != nil && resp.Parameters.RetryAfter > 0 {
			wait = time.Duration(resp.Parameters.RetryAfter) * time.Second
		}
		if wait > c.maxWait {
			return resp, nil
		}
		// В URL есть токен бота, поэтому в лог идёт только имя метода.
		log.Printf("Telegram ограничил частоту запросов %s, повтор через %s (попытка %d из %d)",
			path.Base(url), wait, attempt+1, c.attempts)
		if c.sleep(ctx, wait) != nil {
			return resp, nil
		}
	}
}

// sleepContext ждёт d или отмены ctx.
func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
