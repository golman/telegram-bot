package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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
