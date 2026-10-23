package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	ta "github.com/mymmrac/telego/telegoapi"
)

// stubCaller отвечает заранее заданными ответами по порядку и считает вызовы.
type stubCaller struct {
	responses []stubResponse
	calls     int
}

type stubResponse struct {
	resp *ta.Response
	err  error
}

func (s *stubCaller) Call(_ context.Context, _ string, _ *ta.RequestData) (*ta.Response, error) {
	r := s.responses[min(s.calls, len(s.responses)-1)]
	s.calls++
	return r.resp, r.err
}

func okResponse() stubResponse {
	return stubResponse{resp: &ta.Response{Ok: true, Result: []byte(`true`)}}
}

func apiError(code, retryAfter int) stubResponse {
	e := &ta.Error{ErrorCode: code, Description: "Too Many Requests: retry later"}
	if retryAfter > 0 {
		e.Parameters = &ta.ResponseParameters{RetryAfter: retryAfter}
	}
	return stubResponse{resp: &ta.Response{Ok: false, Error: e}}
}

func TestRateLimitCaller(t *testing.T) {
	errNetwork := errors.New("connection reset")
	tests := []struct {
		name      string
		responses []stubResponse
		data      *ta.RequestData
		sleepErr  error
		wantCalls int
		wantSleep []time.Duration
		wantOk    bool
		wantErr   error
	}{
		{
			name:      "429, затем успех",
			responses: []stubResponse{apiError(429, 3), okResponse()},
			wantCalls: 2, wantSleep: []time.Duration{3 * time.Second}, wantOk: true,
		},
		{
			name:      "429 без retry_after ждёт секунду",
			responses: []stubResponse{apiError(429, 0), okResponse()},
			wantCalls: 2, wantSleep: []time.Duration{time.Second}, wantOk: true,
		},
		{
			name:      "429 на всех попытках",
			responses: []stubResponse{apiError(429, 1)},
			wantCalls: 3, wantSleep: []time.Duration{time.Second, time.Second},
		},
		{
			name:      "retry_after больше максимума — без ожидания",
			responses: []stubResponse{apiError(429, 31)},
			wantCalls: 1,
		},
		{
			name:      "400 не повторяется",
			responses: []stubResponse{apiError(400, 0)},
			wantCalls: 1,
		},
		{
			// Запрос мог дойти до Telegram: повтор опубликовал бы объявление дважды.
			name:      "сетевая ошибка не повторяется",
			responses: []stubResponse{{err: errNetwork}},
			wantCalls: 1, wantErr: errNetwork,
		},
		{
			name:      "тело-поток прочитано и не повторяется",
			responses: []stubResponse{apiError(429, 1), okResponse()},
			data:      &ta.RequestData{BodyStream: bytes.NewReader([]byte("x"))},
			wantCalls: 1,
		},
		{
			name:      "ожидание прервано отменой",
			responses: []stubResponse{apiError(429, 1), okResponse()},
			sleepErr:  context.Canceled,
			wantCalls: 1, wantSleep: []time.Duration{time.Second},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLog(t)
			next := &stubCaller{responses: tt.responses}
			c := newRateLimitCaller(next)
			var slept []time.Duration
			c.sleep = func(_ context.Context, d time.Duration) error {
				slept = append(slept, d)
				return tt.sleepErr
			}
			data := tt.data
			if data == nil {
				data = &ta.RequestData{BodyRaw: []byte(`{}`)}
			}

			resp, err := c.Call(context.Background(), "https://api.telegram.org/bot123:SECRET/sendMessage", data)

			if next.calls != tt.wantCalls {
				t.Errorf("запросов %d, ожидалось %d", next.calls, tt.wantCalls)
			}
			if !reflect.DeepEqual(slept, tt.wantSleep) {
				t.Errorf("ожидания %v, ожидалось %v", slept, tt.wantSleep)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("ошибка %v, ожидалась %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				if resp == nil {
					t.Fatal("ответ nil")
				}
				if resp.Ok != tt.wantOk {
					t.Errorf("ответ Ok = %v, ожидалось %v (%v)", resp.Ok, tt.wantOk, resp)
				}
			}
			if strings.Contains(logs.String(), "SECRET") {
				t.Errorf("в лог попал токен из URL:\n%s", logs.String())
			}
		})
	}
}
