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
			logs := captureLog(t)
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
			logs := captureLog(t)
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

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		raw       string
		wantDebug bool
		wantWarn  bool
	}{
		{"", false, false},
		{"info", false, false},
		{"INFO", false, false},
		{"debug", true, false},
		{" Debug ", true, false},
		{"trace", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			logs := captureLog(t)
			if got := parseLogLevel(tt.raw); got != tt.wantDebug {
				t.Errorf("parseLogLevel(%q) = %v, ожидалось %v", tt.raw, got, tt.wantDebug)
			}
			if got := strings.Contains(logs.String(), "LOG_LEVEL"); got != tt.wantWarn {
				t.Errorf("предупреждение в логе: %v, ожидалось %v:\n%s", got, tt.wantWarn, logs.String())
			}
		})
	}
}

// TestEnvHelpers: пустая переменная даёт значение по умолчанию молча,
// некорректная — значение по умолчанию и предупреждение в лог с именем
// переменной, корректная разбирается. Пробелы по краям игнорируются.
func TestEnvHelpers(t *testing.T) {
	const key = "TEST_TELEGRAM_BOT_ENV"
	tests := []struct {
		name     string
		raw      string
		get      func() any
		want     any
		wantWarn bool
	}{
		{"строка: пусто", "", func() any { return getEnvOrDefault(key, "def") }, "def", false},
		{"строка: только пробелы", "   ", func() any { return getEnvOrDefault(key, "def") }, "def", false},
		{"строка: значение с пробелами", "  abc ", func() any { return getEnvOrDefault(key, "def") }, "abc", false},

		{"int: пусто", "", func() any { return getEnvIntOrDefault(key, 5) }, 5, false},
		{"int: число", " 12 ", func() any { return getEnvIntOrDefault(key, 5) }, 12, false},
		{"int: не число", "abc", func() any { return getEnvIntOrDefault(key, 5) }, 5, true},
		// Размеры и сроки логов должны быть положительными.
		{"int: ноль", "0", func() any { return getEnvIntOrDefault(key, 5) }, 5, true},
		{"int: отрицательное", "-3", func() any { return getEnvIntOrDefault(key, 5) }, 5, true},

		// ID каналов отрицательные и не помещаются в int32.
		{"int64: ID канала", "-1001234567890", func() any { return getEnvInt64OrDefault(key, 0) }, int64(-1001234567890), false},
		{"int64: пусто", "", func() any { return getEnvInt64OrDefault(key, 7) }, int64(7), false},
		{"int64: не число", "-100abc", func() any { return getEnvInt64OrDefault(key, 7) }, int64(7), true},

		{"bool: пусто", "", func() any { return getEnvBoolOrDefault(key, true) }, true, false},
		{"bool: false", "false", func() any { return getEnvBoolOrDefault(key, true) }, false, false},
		{"bool: 0", "0", func() any { return getEnvBoolOrDefault(key, true) }, false, false},
		{"bool: TRUE", " TRUE ", func() any { return getEnvBoolOrDefault(key, false) }, true, false},
		{"bool: не bool", "да", func() any { return getEnvBoolOrDefault(key, true) }, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLog(t)
			t.Setenv(key, tt.raw)

			if got := tt.get(); got != tt.want {
				t.Errorf("значение %v (%T), ожидалось %v (%T)", got, got, tt.want, tt.want)
			}
			if got := strings.Contains(logs.String(), key); got != tt.wantWarn {
				t.Errorf("предупреждение в логе: %v, ожидалось %v:\n%s", got, tt.wantWarn, logs.String())
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
			captureLog(t) // восстановит slog.Default после теста
			t.Cleanup(func() { logLevel.Set(slog.LevelInfo) })
			file := filepath.Join(t.TempDir(), "logs", "bot.log")
			t.Setenv("LOG_FILE_PATH", file)
			t.Setenv("LOG_LEVEL", tt.level)

			debug, err := setupLogging()
			if err != nil {
				t.Fatalf("setupLogging: %v", err)
			}
			slog.Debug("проверка отладки")

			if debug != tt.wantDebug {
				t.Errorf("debug = %v, ожидалось %v", debug, tt.wantDebug)
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
			if got := strings.Contains(out, "некорректный LOG_LEVEL"); got != tt.wantWarn {
				t.Errorf("предупреждение о LOG_LEVEL в файле: %v, ожидалось %v:\n%s", got, tt.wantWarn, out)
			}
		})
	}
}

// TestSetupLoggingBadConfigWarning: предупреждение о некорректной LOG_*
// переменной пишется в формате бота, а не в формате пакета log.
func TestSetupLoggingBadConfigWarning(t *testing.T) {
	captureLog(t)
	t.Cleanup(func() { logLevel.Set(slog.LevelInfo) })
	t.Setenv("LOG_FILE_PATH", filepath.Join(t.TempDir(), "bot.log"))
	t.Setenv("LOG_MAX_BACKUPS", "abc")

	stdout := captureStdout(t, func() {
		if _, err := setupLogging(); err != nil {
			t.Fatalf("setupLogging: %v", err)
		}
	})
	if !regexp.MustCompile(`(?m)^time=\S+Z level=WARN msg="некорректная переменная окружения.*var=LOG_MAX_BACKUPS`).MatchString(stdout) {
		t.Errorf("нет предупреждения о LOG_MAX_BACKUPS в формате бота:\n%s", stdout)
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
