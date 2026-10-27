package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/mymmrac/telego"
)

// testChannelID — канал бота в тестах.
const testChannelID int64 = -1001234567890

// testMediaGroupMessageID — ID первого сообщения альбома в канале,
// который фейковый API возвращает на sendMediaGroup.
const testMediaGroupMessageID int64 = 900

// apiCall — один запрос к фейковому Bot API: имя метода и тело в JSON.
type apiCall struct {
	method string
	body   map[string]any
}

// fakeAPI — фейковый сервер Bot API. Запоминает все запросы по порядку
// и отвечает ошибкой 400 на методы из fail (метод → description).
// Для методов из results вместо ответа по умолчанию отдаётся заданный
// JSON в поле result (метод → JSON), см. respond. На sendMediaGroup
// по умолчанию отдаётся массив из одного сообщения в канале с ID
// testMediaGroupMessageID. Если метод есть и в fail,
// и в results, побеждает fail: отвечается ошибка.
type fakeAPI struct {
	mu      sync.Mutex
	calls   []apiCall
	fail    map[string]string
	results map[string]string
}

// respond задаёт JSON, который фейковый API вернёт в result на метод m.
// Ошибка из fail для того же метода важнее и отменяет этот ответ.
func (f *fakeAPI) respond(m, resultJSON string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.results == nil {
		f.results = map[string]string{}
	}
	f.results[m] = resultJSON
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method := path.Base(r.URL.Path)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// UseNumber, чтобы ID каналов сравнивались точно, а не через float64.
	var body map[string]any
	if len(raw) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	f.mu.Lock()
	f.calls = append(f.calls, apiCall{method: method, body: body})
	desc, failing := f.fail[method]
	result, custom := f.results[method]
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if failing {
		resp, _ := json.Marshal(map[string]any{"ok": false, "error_code": 400, "description": desc})
		_, _ = w.Write(resp)
		return
	}
	if custom {
		_, _ = w.Write([]byte(`{"ok":true,"result":` + result + `}`))
		return
	}
	switch method {
	case "answerCallbackQuery":
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	case "sendMediaGroup":
		// Bot API отвечает на sendMediaGroup массивом сообщений, а не объектом.
		_, _ = w.Write([]byte(fmt.Sprintf(`{"ok":true,"result":[{"message_id":%d,"date":1,"chat":{"id":%d,"type":"channel"}}]}`,
			testMediaGroupMessageID, testChannelID)))
	default:
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":1,"type":"private"}}}`))
	}
}

// allCalls возвращает копию всех запросов в порядке поступления.
func (f *fakeAPI) allCalls() []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]apiCall(nil), f.calls...)
}

// byMethod возвращает запросы к методу m в порядке поступления.
func (f *fakeAPI) byMethod(m string) []apiCall {
	var out []apiCall
	for _, c := range f.allCalls() {
		if c.method == m {
			out = append(out, c)
		}
	}
	return out
}

// newTestBot создаёт VBBot, который ходит в фейковый Bot API.
func newTestBot(t *testing.T, fail map[string]string) (*VBBot, *fakeAPI) {
	t.Helper()
	f := &fakeAPI{fail: fail}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	// Токен должен проходить проверку формата telego.
	b, err := tgbotapi.NewBot("123456:"+strings.Repeat("A", 35),
		tgbotapi.WithAPIServer(srv.URL), tgbotapi.WithDiscardLogger())
	if err != nil {
		t.Fatalf("не удалось создать бота: %v", err)
	}
	return &VBBot{tgbot: b, channelId: testChannelID}, f
}

// jsonField возвращает поле key из JSON-объекта m, приведённое к T.
// Если поля нет или тип другой, тест падает.
func jsonField[T any](t *testing.T, m map[string]any, key string) T {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("в запросе нет поля %q: %v", key, m)
	}
	typed, ok := v.(T)
	if !ok {
		var zero T
		t.Fatalf("поле %q: тип %T, ожидался %T (значение %v)", key, v, zero, v)
	}
	return typed
}

// jsonString возвращает строковое поле key.
func jsonString(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	return jsonField[string](t, m, key)
}

// jsonInt возвращает целочисленное поле key.
func jsonInt(t *testing.T, m map[string]any, key string) int64 {
	t.Helper()
	n := jsonField[json.Number](t, m, key)
	v, err := n.Int64()
	if err != nil {
		t.Fatalf("поле %q: %q не целое число: %v", key, n, err)
	}
	return v
}

// jsonIndex возвращает элемент i JSON-массива arr как T.
func jsonIndex[T any](t *testing.T, arr []any, i int) T {
	t.Helper()
	if i >= len(arr) {
		t.Fatalf("в массиве %d элементов, нужен индекс %d: %v", len(arr), i, arr)
	}
	typed, ok := arr[i].(T)
	if !ok {
		var zero T
		t.Fatalf("элемент %d: тип %T, ожидался %T (значение %v)", i, arr[i], zero, arr[i])
	}
	return typed
}

// onlyButton возвращает единственную inline-кнопку из reply_markup запроса.
func onlyButton(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	markup := jsonField[map[string]any](t, body, "reply_markup")
	rows := jsonField[[]any](t, markup, "inline_keyboard")
	if len(rows) != 1 {
		t.Fatalf("ожидался один ряд кнопок, получено %d: %v", len(rows), rows)
	}
	row := jsonIndex[[]any](t, rows, 0)
	if len(row) != 1 {
		t.Fatalf("ожидалась одна кнопка в ряду, получено %d: %v", len(row), row)
	}
	return jsonIndex[map[string]any](t, row, 0)
}

// jsonEntities возвращает массив entities из поля key, разобранный
// в []tgbotapi.MessageEntity, чтобы сравнивать его со структурами.
func jsonEntities(t *testing.T, m map[string]any, key string) []tgbotapi.MessageEntity {
	t.Helper()
	raw, err := json.Marshal(jsonField[[]any](t, m, key))
	if err != nil {
		t.Fatalf("поле %q: не удалось сериализовать: %v", key, err)
	}
	var out []tgbotapi.MessageEntity
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("поле %q: не удалось разобрать entities: %v", key, err)
	}
	return out
}

// authorTexts возвращает тексты всех sendMessage в личный чат автора по порядку.
func authorTexts(t *testing.T, f *fakeAPI) []string {
	t.Helper()
	var texts []string
	for _, c := range f.byMethod("sendMessage") {
		if jsonInt(t, c.body, "chat_id") == testAuthorID {
			texts = append(texts, jsonString(t, c.body, "text"))
		}
	}
	return texts
}

// channelCalls считает вызовы метода m с chat_id канала.
func channelCalls(t *testing.T, f *fakeAPI, m string) int {
	t.Helper()
	n := 0
	for _, c := range f.byMethod(m) {
		if jsonInt(t, c.body, "chat_id") == testChannelID {
			n++
		}
	}
	return n
}
