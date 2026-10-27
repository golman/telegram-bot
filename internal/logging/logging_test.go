package logging_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/config"
	"github.com/golman/telegram-bot/internal/logging"
	"github.com/golman/telegram-bot/internal/testutil"
)

const testToken = "123456:SECRETSECRETSECRETSECRETSECRETSECRE"

func TestTelegoLogger(t *testing.T) {
	tests := []struct {
		name      string
		debug     bool
		log       func(l *logging.TelegoLogger)
		wantLog   bool
		wantInLog string
	}{
		// Ошибки — на уровне ERROR: на уровне info они не должны теряться.
		{"ошибка пишется всегда", false, func(l *logging.TelegoLogger) { l.Errorf("Getting updates: %s", "timeout") }, true, `level=ERROR msg="telego: Getting updates: timeout"`},
		{"debug скрыт на уровне info", false, func(l *logging.TelegoLogger) { l.Debugf("API response %s: %s", "getUpdates", "Иван") }, false, ""},
		{"debug пишется на уровне debug", true, func(l *logging.TelegoLogger) { l.Debugf("API response %s: %s", "getUpdates", "Иван") }, true, `level=DEBUG msg="telego: API response getUpdates: Иван"`},
		{"токен в ошибке скрыт", false, func(l *logging.TelegoLogger) {
			l.Errorf("call to %q", "https://api.telegram.org/bot"+testToken+"/getMe")
		}, true, "BOT_TOKEN"},
		{"токен в debug скрыт", true, func(l *logging.TelegoLogger) {
			l.Debugf("API call to: %q", "https://api.telegram.org/bot"+testToken+"/getMe")
		}, true, "BOT_TOKEN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := testutil.CaptureLog(t)
			tt.log(logging.NewTelegoLogger(testToken, tt.debug))

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
			srv := httptest.NewServer(http.HandlerFunc(okAPI))
			t.Cleanup(srv.Close)
			bot, err := telego.NewBot(testToken, append(logging.BotOptions(testToken, tt.debug), telego.WithAPIServer(srv.URL))...)
			if err != nil {
				t.Fatalf("не удалось создать бота: %v", err)
			}

			_, _ = bot.SendMessage(context.Background(), &telego.SendMessageParams{ChatID: telego.ChatID{ID: 1}, Text: "Продам велосипед"})

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

// TestSetup: логи идут в stdout и в файл в формате key=value со временем
// в UTC, cfg.Debug управляет уровнем.
func TestSetup(t *testing.T) {
	tests := []struct {
		name  string
		debug bool
	}{
		{"info", false},
		{"debug", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.CaptureLog(t) // восстановит slog.Default после теста
			t.Cleanup(logging.ResetLevel)
			file := filepath.Join(t.TempDir(), "logs", "bot.log")
			var stdout bytes.Buffer

			if err := logging.Setup(config.Log{FilePath: file, Debug: tt.debug}, &stdout); err != nil {
				t.Fatalf("Setup: %v", err)
			}
			slog.Debug("проверка отладки")

			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("лог-файл не создан: %v", err)
			}
			for where, out := range map[string]string{"файле": string(raw), "stdout": stdout.String()} {
				if !regexp.MustCompile(`(?m)^time=\S+Z level=INFO msg="логирование настроено"`).MatchString(out) {
					t.Errorf("в %s нет записи о настройке в формате key=value со временем в UTC:\n%s", where, out)
				}
				if !strings.Contains(out, "file="+file) {
					t.Errorf("в записи о настройке в %s нет file=%s:\n%s", where, file, out)
				}
				if got := strings.Contains(out, "проверка отладки"); got != tt.debug {
					t.Errorf("debug-запись в %s: %v, ожидалось %v:\n%s", where, got, tt.debug, out)
				}
			}
		})
	}
}

// TestSetupResetsLevel: Setup без Debug возвращает уровень info, даже если
// до этого уровень был debug — уровень не наследуется от прошлой настройки.
func TestSetupResetsLevel(t *testing.T) {
	testutil.CaptureLog(t) // восстановит slog.Default после теста
	t.Cleanup(logging.ResetLevel)
	dir := t.TempDir()
	if err := logging.Setup(config.Log{FilePath: filepath.Join(dir, "debug.log"), Debug: true}, io.Discard); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	var stdout bytes.Buffer
	if err := logging.Setup(config.Log{FilePath: filepath.Join(dir, "info.log")}, &stdout); err != nil {
		t.Fatalf("Setup: %v", err)
	}
	slog.Debug("проверка отладки")

	if out := stdout.String(); strings.Contains(out, "проверка отладки") || !strings.Contains(out, "log_level=INFO") {
		t.Errorf("получен уровень debug после Setup без Debug, ожидался info:\n%s", out)
	}
}

// TestSetupUnwritableFile: если файл логов нельзя открыть на запись, Setup
// возвращает ошибку сразу, а не молча пишет только в stdout, и вывод остаётся
// тем, что настроил Bootstrap.
func TestSetupUnwritableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root пишет и в каталог без права записи")
	}
	testutil.CaptureLog(t) // восстановит slog.Default после теста
	var stdout bytes.Buffer
	logging.Bootstrap(&stdout)
	dir := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(dir, 0o555); err != nil {
		t.Fatal(err)
	}

	err := logging.Setup(config.Log{FilePath: filepath.Join(dir, "bot.log")}, io.Discard)
	if err == nil {
		t.Fatal("получено: Setup без ошибки для каталога без права записи, ожидалась ошибка")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "bot.log")); statErr == nil {
		t.Error("получен файл логов в каталоге без права записи, ожидалось его отсутствие")
	}
	slog.Info("после ошибки настройки")
	if !strings.Contains(stdout.String(), "после ошибки настройки") {
		t.Errorf("получено без записи в stdout после ошибки Setup, ожидалась запись:\n%s", stdout.String())
	}
}

// TestBootstrap: до Setup логи идут в stdout в формате бота с уровнем info,
// и туда же — если Setup не смог открыть файл.
func TestBootstrap(t *testing.T) {
	testutil.CaptureLog(t) // восстановит slog.Default после теста
	var stdout bytes.Buffer
	logging.Bootstrap(&stdout)

	slog.Debug("проверка отладки")
	slog.Info("до настройки файла")
	// Каталог логов внутри обычного файла создать нельзя.
	notDir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notDir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := logging.Setup(config.Log{FilePath: filepath.Join(notDir, "logs", "bot.log")}, io.Discard); err == nil {
		t.Fatal("Setup с каталогом внутри файла: ошибки нет, ожидалась")
	}
	slog.Warn("после ошибки настройки")

	out := stdout.String()
	for _, msg := range []string{"до настройки файла", "после ошибки настройки"} {
		if !regexp.MustCompile(`(?m)^time=\S+Z level=(INFO|WARN) msg="` + msg + `"`).MatchString(out) {
			t.Errorf("в stdout нет записи %q в формате бота:\n%s", msg, out)
		}
	}
	if strings.Contains(out, "проверка отладки") {
		t.Errorf("debug-запись в stdout до Setup, ожидался уровень info:\n%s", out)
	}
}

// okAPI — минимальный фейковый Bot API: на любой метод отвечает успехом
// с сообщением в результате.
func okAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"}}}`))
}
