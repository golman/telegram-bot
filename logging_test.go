package main

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tgbotapi "github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/testutil"
)

const testToken = "123456:SECRETSECRETSECRETSECRETSECRETSECRE"

func TestTelegoLogger(t *testing.T) {
	tests := []struct {
		name      string
		debug     bool
		log       func(l *telegoLogger)
		wantLog   bool
		wantInLog string
	}{
		// Ошибки — на уровне ERROR: на уровне info они не должны теряться.
		{"ошибка пишется всегда", false, func(l *telegoLogger) { l.Errorf("Getting updates: %s", "timeout") }, true, `level=ERROR msg="telego: Getting updates: timeout"`},
		{"debug скрыт на уровне info", false, func(l *telegoLogger) { l.Debugf("API response %s: %s", "getUpdates", "Иван") }, false, ""},
		{"debug пишется на уровне debug", true, func(l *telegoLogger) { l.Debugf("API response %s: %s", "getUpdates", "Иван") }, true, `level=DEBUG msg="telego: API response getUpdates: Иван"`},
		{"токен в ошибке скрыт", false, func(l *telegoLogger) { l.Errorf("call to %q", "https://api.telegram.org/bot"+testToken+"/getMe") }, true, "BOT_TOKEN"},
		{"токен в debug скрыт", true, func(l *telegoLogger) { l.Debugf("API call to: %q", "https://api.telegram.org/bot"+testToken+"/getMe") }, true, "BOT_TOKEN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := testutil.CaptureLog(t)
			tt.log(newTelegoLogger(testToken, tt.debug))

			out := logs.String()
			if got := out != ""; got != tt.wantLog {
				t.Fatalf("запись в логе: %v, ожидалось %v:\n%s", got, tt.wantLog, out)
			}
			if !strings.Contains(out, tt.wantInLog) {
				t.Errorf("в логе нет %q:\n%s", tt.wantInLog, out)
			}
			if strings.Contains(out, "SECRET") {
				t.Errorf("в лог попал токен:\n%s", out)
			}
		})
	}
}

// TestBotOptionsLogging: логи telego идут в общий лог бота, а не в stderr.
// На уровне info запросы и их тела (тексты объявлений) не пишутся,
// на уровне debug пишутся, но без токена.
func TestBotOptionsLogging(t *testing.T) {
	tests := []struct {
		name     string
		debug    bool
		wantBody bool
	}{
		{"info", false, false},
		{"debug", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := testutil.CaptureLog(t)
			srv := httptest.NewServer(&fakeAPI{})
			t.Cleanup(srv.Close)
			bot, err := tgbotapi.NewBot(testToken, append(botOptions(testToken, tt.debug), tgbotapi.WithAPIServer(srv.URL))...)
			if err != nil {
				t.Fatalf("не удалось создать бота: %v", err)
			}

			_, _ = bot.SendMessage(context.Background(), &tgbotapi.SendMessageParams{ChatID: tgbotapi.ChatID{ID: 1}, Text: "Продам велосипед"})

			out := logs.String()
			if got := strings.Contains(out, "Продам велосипед"); got != tt.wantBody {
				t.Errorf("тело запроса в логе: %v, ожидалось %v:\n%s", got, tt.wantBody, out)
			}
			if tt.debug && !strings.Contains(out, "telego: API call to") {
				t.Errorf("в логе нет debug-записи telego о запросе:\n%s", out)
			}
			if strings.Contains(out, "SECRET") {
				t.Errorf("в лог попал токен:\n%s", out)
			}
		})
	}
}

// TestSetupLogging: логи идут в файл в формате key=value со временем в UTC,
// LOG_LEVEL управляет уровнем, а предупреждение о некорректном LOG_LEVEL
// попадает в файл, а не теряется до настройки вывода.
func TestSetupLogging(t *testing.T) {
	tests := []struct {
		level     string
		wantDebug bool
		wantWarn  bool
	}{
		{"", false, false},
		{"debug", true, false},
		{"trace", false, true},
	}
	for _, tt := range tests {
		t.Run("LOG_LEVEL="+tt.level, func(t *testing.T) {
			testutil.CaptureLog(t) // восстановит slog.Default после теста
			t.Cleanup(func() { logLevel.Set(slog.LevelInfo) })
			file := filepath.Join(t.TempDir(), "logs", "bot.log")
			env := testEnv(map[string]string{"LOG_FILE_PATH": file, "LOG_LEVEL": tt.level})

			cfg, err := initLogging(env)
			if err != nil {
				t.Fatalf("initLogging: %v", err)
			}
			slog.Debug("проверка отладки")

			if cfg.Log.Debug != tt.wantDebug {
				t.Errorf("получено debug = %v, ожидалось %v", cfg.Log.Debug, tt.wantDebug)
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("лог-файл не создан: %v", err)
			}
			out := string(raw)
			if !regexp.MustCompile(`(?m)^time=\S+Z level=INFO msg="логирование настроено"`).MatchString(out) {
				t.Errorf("нет записи о настройке в формате key=value со временем в UTC:\n%s", out)
			}
			if got := strings.Contains(out, "проверка отладки"); got != tt.wantDebug {
				t.Errorf("debug-запись в файле: %v, ожидалось %v:\n%s", got, tt.wantDebug, out)
			}
			badLevel := regexp.MustCompile(`(?m)^time=\S+Z level=WARN msg="некорректная переменная окружения.*var=LOG_LEVEL`)
			if got := badLevel.MatchString(out); got != tt.wantWarn {
				t.Errorf("предупреждение о LOG_LEVEL в файле: %v, ожидалось %v:\n%s", got, tt.wantWarn, out)
			}
		})
	}
}

// TestSetupLoggingBadConfigWarning: предупреждение о некорректной LOG_*
// переменной пишется в формате бота, а не в формате пакета log.
func TestSetupLoggingBadConfigWarning(t *testing.T) {
	testutil.CaptureLog(t)
	t.Cleanup(func() { logLevel.Set(slog.LevelInfo) })
	env := testEnv(map[string]string{"LOG_FILE_PATH": filepath.Join(t.TempDir(), "bot.log"), "LOG_MAX_BACKUPS": "abc"})

	stdout := captureStdout(t, func() {
		if _, err := initLogging(env); err != nil {
			t.Fatalf("initLogging: %v", err)
		}
	})
	if !regexp.MustCompile(`(?m)^time=\S+Z level=WARN msg="некорректная переменная окружения.*var=LOG_MAX_BACKUPS`).MatchString(stdout) {
		t.Errorf("нет предупреждения о LOG_MAX_BACKUPS в формате бота:\n%s", stdout)
	}
}

// testEnv — подставной getenv: BOT_TOKEN и CHANNEL_ID заданы, остальное из vars.
func testEnv(vars map[string]string) func(string) string {
	return func(key string) string {
		switch key {
		case "BOT_TOKEN":
			return testToken
		case "CHANNEL_ID":
			return "-100"
		}
		return vars[key]
	}
}

// captureStdout перехватывает os.Stdout на время fn и возвращает вывод.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	prev := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stdout = prev
	w.Close()
	return <-done
}
