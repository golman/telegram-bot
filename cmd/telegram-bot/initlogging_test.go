package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/golman/telegram-bot/internal/testutil"
)

// TestInitLoggingWarningInFile: предупреждение о некорректном LOG_LEVEL
// попадает в файл, а не теряется до настройки вывода.
func TestInitLoggingWarningInFile(t *testing.T) {
	testutil.CaptureLog(t) // восстановит slog.Default после теста
	file := filepath.Join(t.TempDir(), "logs", "bot.log")
	env := testEnv(map[string]string{"LOG_FILE_PATH": file, "LOG_LEVEL": "trace"})

	cfg, err := initLogging(env, io.Discard)
	if err != nil {
		t.Fatalf("initLogging: %v", err)
	}
	if cfg.Log.Debug {
		t.Errorf("получено debug = true, ожидалось false")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("лог-файл не создан: %v", err)
	}
	out := string(raw)
	badLevel := regexp.MustCompile(`(?m)^time=\S+Z level=WARN msg="некорректная переменная окружения.*var=LOG_LEVEL`)
	if !badLevel.MatchString(out) {
		t.Errorf("нет предупреждения о LOG_LEVEL в файле:\n%s", out)
	}
}

// TestInitLoggingBadConfigWarning: предупреждение о некорректной LOG_*
// переменной пишется в stdout в формате бота, а не в формате пакета log.
func TestInitLoggingBadConfigWarning(t *testing.T) {
	testutil.CaptureLog(t)
	env := testEnv(map[string]string{"LOG_FILE_PATH": filepath.Join(t.TempDir(), "bot.log"), "LOG_MAX_BACKUPS": "abc"})

	var stdout bytes.Buffer
	if _, err := initLogging(env, &stdout); err != nil {
		t.Fatalf("initLogging: %v", err)
	}
	if !regexp.MustCompile(`(?m)^time=\S+Z level=WARN msg="некорректная переменная окружения.*var=LOG_MAX_BACKUPS`).MatchString(stdout.String()) {
		t.Errorf("нет предупреждения о LOG_MAX_BACKUPS в формате бота:\n%s", stdout.String())
	}
}

// testEnv — подставной getenv: BOT_TOKEN и CHANNEL_ID заданы, остальное из vars.
func testEnv(vars map[string]string) func(string) string {
	return func(key string) string {
		switch key {
		case "BOT_TOKEN":
			return "123456:SECRETSECRETSECRETSECRETSECRETSECRE"
		case "CHANNEL_ID":
			return "-100"
		}
		return vars[key]
	}
}
