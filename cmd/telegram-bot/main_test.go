package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/testutil"
)

// testToken — фейковый токен; в логах он встречаться не должен.
const testToken = "123456:SECRETSECRETSECRETSECRETSECRETSECRE"

// Записи, которые пишет run при ошибках конфига.
var (
	logSetUp       = regexp.MustCompile(`(?m)^time=\S+Z level=INFO msg="логирование настроено".* log_level=INFO`)
	errNoToken     = regexp.MustCompile(`(?m)^time=\S+Z level=ERROR msg="не задан BOT_TOKEN"$`)
	errNoChannel   = regexp.MustCompile(`(?m)^time=\S+Z level=ERROR msg="не задан или некорректен CHANNEL_ID"$`)
	errLogSetup    = regexp.MustCompile(`(?m)^time=\S+Z level=ERROR msg="не удалось настроить логирование" err=`)
	warnLogLevel   = badVarWarning("LOG_LEVEL")
	warnMaxBackups = badVarWarning("LOG_MAX_BACKUPS")
	warnChannelID  = badVarWarning("CHANNEL_ID")
)

// badVarWarning — запись в формате бота о некорректной переменной name.
func badVarWarning(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^time=\S+Z level=WARN msg="некорректная переменная окружения, используется значение по умолчанию" var=` + name + ` `)
}

// TestRunLogsConfigWarningsToFile: предупреждение о некорректном LOG_LEVEL
// попадает и в файл, и в stdout — после настройки логов и до записи об ошибке
// конфига, с которой run завершается с кодом 1.
func TestRunLogsConfigWarningsToFile(t *testing.T) {
	testutil.CaptureLog(t) // восстановит slog.Default после теста
	file := filepath.Join(t.TempDir(), "logs", "bot.log")
	env := testEnv(map[string]string{"LOG_FILE_PATH": file, "LOG_LEVEL": "trace", "CHANNEL_ID": "-100"})

	var stdout bytes.Buffer
	if code := run(env, &stdout); code != 1 {
		t.Fatalf("получен код %d, ожидался 1", code)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("лог-файл не создан: %v", err)
	}
	for name, out := range map[string]string{"файл": string(raw), "stdout": stdout.String()} {
		assertInOrder(t, name, out, logSetUp, warnLogLevel, errNoToken)
	}
}

// TestRunConfigErrors: при ошибке конфига или настройки логов run пишет
// запись ERROR и предупреждения конфига в stdout и возвращает код 1.
func TestRunConfigErrors(t *testing.T) {
	cases := []struct {
		name        string
		vars        map[string]string
		blockLogDir bool             // каталог логов лежал бы внутри обычного файла
		want        []*regexp.Regexp // записи stdout в порядке появления
	}{
		{
			name: "нет BOT_TOKEN",
			vars: map[string]string{"CHANNEL_ID": "-100", "LOG_MAX_BACKUPS": "abc"},
			want: []*regexp.Regexp{logSetUp, warnMaxBackups, errNoToken},
		},
		{
			name: "некорректный CHANNEL_ID",
			vars: map[string]string{"BOT_TOKEN": testToken, "CHANNEL_ID": "abc"},
			want: []*regexp.Regexp{logSetUp, warnChannelID, errNoChannel},
		},
		{
			name: "нет ни BOT_TOKEN, ни CHANNEL_ID",
			vars: map[string]string{},
			want: []*regexp.Regexp{logSetUp, errNoToken},
		},
		{
			name:        "каталог логов не создать",
			vars:        map[string]string{"BOT_TOKEN": testToken, "CHANNEL_ID": "-100", "LOG_LEVEL": "trace"},
			blockLogDir: true,
			want:        []*regexp.Regexp{warnLogLevel, errLogSetup},
		},
		{
			// Ошибка настройки логов важнее ошибки конфига.
			name:        "каталог логов не создать и нет BOT_TOKEN",
			vars:        map[string]string{"CHANNEL_ID": "-100"},
			blockLogDir: true,
			want:        []*regexp.Regexp{errLogSetup},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testutil.CaptureLog(t)
			dir := t.TempDir()
			tc.vars["LOG_FILE_PATH"] = filepath.Join(dir, "bot.log")
			if tc.blockLogDir {
				blocker := filepath.Join(dir, "file")
				if err := os.WriteFile(blocker, nil, 0o644); err != nil {
					t.Fatal(err)
				}
				tc.vars["LOG_FILE_PATH"] = filepath.Join(blocker, "logs", "bot.log")
			}

			var stdout bytes.Buffer
			if code := run(testEnv(tc.vars), &stdout); code != 1 {
				t.Fatalf("получен код %d, ожидался 1", code)
			}
			out := stdout.String()
			assertInOrder(t, "stdout", out, tc.want...)
			if tc.blockLogDir {
				if logSetUp.MatchString(out) {
					t.Errorf("получена запись «логирование настроено», ожидалось её отсутствие:\n%s", out)
				}
				if errNoToken.MatchString(out) {
					t.Errorf("получена запись «не задан BOT_TOKEN», ожидалась только ошибка настройки логов:\n%s", out)
				}
			}
			if strings.Contains(out, "SECRET") {
				t.Errorf("получен токен в stdout, ожидалось его отсутствие:\n%s", out)
			}
		})
	}
}

// assertInOrder проверяет, что в out есть записи want, причём в этом порядке.
func assertInOrder(t *testing.T, name, out string, want ...*regexp.Regexp) {
	t.Helper()
	pos := 0
	for _, re := range want {
		loc := re.FindStringIndex(out[pos:])
		if loc == nil {
			t.Errorf("%s: получено без записи %q после позиции %d, ожидалась запись:\n%s", name, re, pos, out)
			return
		}
		pos += loc[1]
	}
}

// testEnv — подставной getenv из vars.
func testEnv(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

// TestRunServesUpdates: run с настоящими telego и tgapi.NewCaller поверх
// фейкового Bot API. Первый getUpdates получает 429 и повторяется после
// retry_after (без NewCaller telego ждал бы 8 секунд), объявление из второго
// ответа публикуется в канал, а после отмены ctx run возвращает 0.
func TestRunServesUpdates(t *testing.T) {
	testutil.CaptureLog(t)
	api := newRunAPI()
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)
	env := testEnv(map[string]string{
		"BOT_TOKEN":        testToken,
		"CHANNEL_ID":       "-100",
		"BOT_AUTH_ENABLED": "false",
		"LOG_FILE_PATH":    filepath.Join(t.TempDir(), "bot.log"),
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stdout syncBuffer
	done := make(chan int, 1)
	go func() { done <- runWith(ctx, env, &stdout, telego.WithAPIServer(srv.URL)) }()

	select {
	case <-api.published:
	case code := <-done:
		t.Fatalf("run вернул %d до публикации:\n%s", code, stdout.String())
	case <-time.After(5 * time.Second):
		t.Fatalf("получено: объявление не опубликовано за 5 с, ожидалась публикация после повтора 429:\n%s", stdout.String())
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("получен код %d, ожидался 0", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("получено: run не вернулся за 5 с после отмены, ожидалась остановка")
	}

	out := stdout.String()
	for _, want := range []string{`msg="Telegram ограничил частоту запросов, повтор" method=getUpdates`, `msg="бот остановлен"`} {
		if !strings.Contains(out, want) {
			t.Errorf("получено без записи %q, ожидалась запись:\n%s", want, out)
		}
	}
	if strings.Contains(out, "SECRET") {
		t.Errorf("получен токен в логе, ожидалось его отсутствие:\n%s", out)
	}
}

// runAPI — фейковый Bot API для TestRunServesUpdates: первый getUpdates
// отвечает 429, второй — одним текстовым объявлением, дальше — пусто.
// published закрывается, когда объявление ушло в канал.
type runAPI struct {
	mu        sync.Mutex
	polls     int
	published chan struct{}
	once      sync.Once
}

func newRunAPI() *runAPI { return &runAPI{published: make(chan struct{})} }

func (a *runAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch path.Base(r.URL.Path) {
	case "getUpdates":
		a.mu.Lock()
		a.polls++
		n := a.polls
		a.mu.Unlock()
		switch n {
		case 1:
			_, _ = io.WriteString(w, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`)
		case 2:
			_, _ = io.WriteString(w, `{"ok":true,"result":[{"update_id":1,"message":{"message_id":10,"date":1,`+
				`"chat":{"id":42,"type":"private"},"from":{"id":42,"is_bot":false,"first_name":"Иван"},"text":"Продам велосипед"}}]}`)
		default:
			// Long polling без новых обновлений; пауза, чтобы цикл не крутился вхолостую.
			time.Sleep(20 * time.Millisecond)
			_, _ = io.WriteString(w, `{"ok":true,"result":[]}`)
		}
	case "sendMessage":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["chat_id"] == float64(-100) {
			a.once.Do(func() { close(a.published) })
		}
		_, _ = io.WriteString(w, `{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":-100,"type":"channel"}}}`)
	default:
		_, _ = io.WriteString(w, `{"ok":true,"result":true}`)
	}
}

// syncBuffer — bytes.Buffer, в который можно писать из горутины run
// и читать из теста.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
