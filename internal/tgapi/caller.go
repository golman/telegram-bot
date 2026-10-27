// Package tgapi оборачивает запросы к Bot API: повтор при 429 и дедлайн каждой попытки.
package tgapi

import (
	"context"
	"log/slog"
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
		slog.Warn("Telegram ограничил частоту запросов, повтор",
			"method", path.Base(url), "retry_after", wait, "attempt", attempt+1, "attempts", c.attempts)
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

// LongPollTimeoutSeconds — сколько Telegram держит запрос getUpdates без новых обновлений.
const LongPollTimeoutSeconds = 8

// apiTimeout — предельное время одной попытки запроса к Bot API. Клиент fasthttp
// в telego создан без таймаута, и без дедлайна зависший запрос остановил бы
// цикл событий навсегда. Должен быть больше long polling, иначе getUpdates
// обрывался бы раньше ответа Telegram.
const apiTimeout = 15 * time.Second

// NewCaller собирает caller для бота: повтор при 429 поверх таймаута,
// чтобы у каждой попытки был свой дедлайн.
func NewCaller() ta.Caller {
	return newRateLimitCaller(newTimeoutCaller(ta.DefaultFastHTTPCaller, apiTimeout))
}

// timeoutCaller ограничивает каждый запрос временем timeout. Используется
// дедлайн контекста: fasthttp в telego прерывает запрос только по дедлайну,
// простую отмену ctx он не замечает.
type timeoutCaller struct {
	next    ta.Caller
	timeout time.Duration
}

func newTimeoutCaller(next ta.Caller, timeout time.Duration) *timeoutCaller {
	return &timeoutCaller{next: next, timeout: timeout}
}

func (c *timeoutCaller) Call(ctx context.Context, url string, data *ta.RequestData) (*ta.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.next.Call(ctx, url, data)
}
