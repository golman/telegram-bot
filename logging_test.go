package main

import (
	"context"
	"net/http/httptest"
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
		{"ошибка пишется всегда", false, func(l *telegoLogger) { l.Errorf("Getting updates: %s", "timeout") }, true, "telego: Getting updates: timeout"},
		{"debug скрыт на уровне info", false, func(l *telegoLogger) { l.Debugf("API response %s: %s", "getUpdates", "Иван") }, false, ""},
		{"debug пишется на уровне debug", true, func(l *telegoLogger) { l.Debugf("API response %s: %s", "getUpdates", "Иван") }, true, "telego: API response getUpdates: Иван"},
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
