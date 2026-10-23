package main

import (
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/mymmrac/telego"
)

func TestStartReturnsWhenUpdatesClosed(t *testing.T) {
	logs := captureLog(t)
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	b := &VBBot{mediamsg: map[string]*MediaMessage{}, ticker: ticker}

	ch := make(chan tgbotapi.Update)
	close(ch)

	done := make(chan struct{})
	go func() {
		b.Start(ch)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start не вернулся после закрытия канала обновлений")
	}
	if out := logs.String(); !strings.Contains(out, "канал обновлений закрыт") {
		t.Fatalf("в логе нет записи о закрытии канала:\n%s", out)
	}
}

// TestDispatchRecoversFromPanic проверяет, что паника в обработчике
// не выходит из dispatch и попадает в лог со стеком.
//
// Источник паники: у VBBot нет клиента (tgbot == nil), а handleCallbackQuery
// в defer всегда вызывает tgbot.AnswerCallbackQuery. Пустые callback data
// не разбираются, обработчик сразу выходит, и defer разыменовывает nil-клиент.
// Если этот путь перестанет паниковать (например, появится проверка
// tgbot != nil) и тест упадёт с «в логе нет паники», замени источник
// паники в тесте, а не чини dispatch.
//
// Запросов к API здесь быть не может: клиента нет, поэтому фейковый API
// не нужен.
func TestDispatchRecoversFromPanic(t *testing.T) {
	logs := captureLog(t)
	b := &VBBot{}
	update := tgbotapi.Update{UpdateID: 7, CallbackQuery: &tgbotapi.CallbackQuery{ID: "q"}}

	b.dispatch(update)

	// Паника должна была произойти и попасть в лог вместе со стеком,
	// иначе тест прошёл бы, даже если обработчик вышел без паники.
	out := logs.String()
	if !strings.Contains(out, "паника при обработке обновления 7") || !strings.Contains(out, "goroutine") {
		t.Fatalf("в логе нет паники со стеком:\n%s", out)
	}
}
