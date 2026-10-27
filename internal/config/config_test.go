package config

import (
	"errors"
	"maps"
	"slices"
	"testing"
)

// TestLoad: пустая переменная даёт значение по умолчанию молча,
// некорректная — значение по умолчанию и Warning с именем переменной,
// исходным значением и умолчанием; корректная разбирается. Пробелы по краям
// игнорируются. При ошибке (нет BOT_TOKEN или CHANNEL_ID) Log и Warning
// всё равно заполнены: main настраивает по ним логи до выхода.
func TestLoad(t *testing.T) {
	// base — минимальное корректное окружение.
	base := map[string]string{"BOT_TOKEN": "123:abc", "CHANNEL_ID": "-100"}
	withBase := func(extra map[string]string) map[string]string {
		env := maps.Clone(base)
		maps.Copy(env, extra)
		return env
	}
	defaultLog := Log{FilePath: "logs/telegram-bot.log", MaxSizeMB: 20, MaxBackups: 5, MaxAgeDays: 14, Compress: true, Debug: false}
	defaults := Config{Token: "123:abc", ChannelID: -100, AuthEnabled: true, Log: defaultLog}
	// with возвращает умолчания с изменением.
	with := func(change func(c *Config)) Config {
		c := defaults
		change(&c)
		return c
	}

	tests := []struct {
		name     string
		env      map[string]string
		want     Config
		warnings []Warning
		wantErr  error
	}{
		{
			name: "всё задано корректно",
			env: map[string]string{
				"BOT_TOKEN": "123:abc", "CHANNEL_ID": "-1001234567890", "BOT_AUTH_ENABLED": "false",
				"LOG_FILE_PATH": "/var/log/bot.log", "LOG_MAX_SIZE_MB": "50", "LOG_MAX_BACKUPS": "3",
				"LOG_MAX_AGE_DAYS": "7", "LOG_COMPRESS": "false", "LOG_LEVEL": "debug",
			},
			want: Config{Token: "123:abc", ChannelID: -1001234567890, AuthEnabled: false, Log: Log{
				FilePath: "/var/log/bot.log", MaxSizeMB: 50, MaxBackups: 3, MaxAgeDays: 7, Compress: false, Debug: true,
			}},
		},
		{name: "умолчания", env: base, want: defaults},
		{
			name: "пробелы по краям обрезаются",
			env:  map[string]string{"BOT_TOKEN": " 123:abc ", "CHANNEL_ID": " -100 "},
			want: defaults,
		},
		{
			name:     "нет BOT_TOKEN: Log и Warning заполнены",
			env:      map[string]string{"CHANNEL_ID": "-100", "LOG_FILE_PATH": "/tmp/bot.log", "LOG_LEVEL": "trace"},
			want:     with(func(c *Config) { c.Token = ""; c.Log.FilePath = "/tmp/bot.log" }),
			warnings: []Warning{{Var: "LOG_LEVEL", Value: "trace", Default: "info"}},
			wantErr:  ErrNoToken,
		},
		// Без обеих обязательных переменных main, как и раньше, пишет «не задан BOT_TOKEN».
		{name: "нет ни BOT_TOKEN, ни CHANNEL_ID", env: map[string]string{}, want: with(func(c *Config) { c.Token = ""; c.ChannelID = 0 }), wantErr: ErrNoToken},
		{name: "BOT_TOKEN из пробелов", env: withBase(map[string]string{"BOT_TOKEN": "   "}), want: with(func(c *Config) { c.Token = "" }), wantErr: ErrNoToken},
		{
			name:     "CHANNEL_ID не число: Log и Warning заполнены",
			env:      withBase(map[string]string{"CHANNEL_ID": "abc", "LOG_MAX_AGE_DAYS": "30"}),
			want:     with(func(c *Config) { c.ChannelID = 0; c.Log.MaxAgeDays = 30 }),
			warnings: []Warning{{Var: "CHANNEL_ID", Value: "abc", Default: int64(0)}},
			wantErr:  ErrNoChannel,
		},
		{name: "CHANNEL_ID ноль", env: withBase(map[string]string{"CHANNEL_ID": "0"}), want: with(func(c *Config) { c.ChannelID = 0 }), wantErr: ErrNoChannel},
		{
			name: "некорректные LOG_*: умолчания и по Warning на переменную",
			env:  withBase(map[string]string{"LOG_MAX_BACKUPS": "0", "LOG_COMPRESS": "да", "LOG_LEVEL": "trace"}),
			want: defaults,
			warnings: []Warning{
				{Var: "LOG_MAX_BACKUPS", Value: "0", Default: 5},
				{Var: "LOG_COMPRESS", Value: "да", Default: true},
				{Var: "LOG_LEVEL", Value: "trace", Default: "info"},
			},
		},

		// Строки.
		{name: "строка: пусто", env: withBase(map[string]string{"LOG_FILE_PATH": ""}), want: defaults},
		{name: "строка: только пробелы", env: withBase(map[string]string{"LOG_FILE_PATH": "   "}), want: defaults},
		{name: "строка: значение с пробелами", env: withBase(map[string]string{"LOG_FILE_PATH": "  abc "}), want: with(func(c *Config) { c.Log.FilePath = "abc" })},

		// Целые числа. Размеры и сроки логов должны быть положительными.
		{name: "int: пусто", env: withBase(map[string]string{"LOG_MAX_SIZE_MB": ""}), want: defaults},
		{name: "int: число", env: withBase(map[string]string{"LOG_MAX_SIZE_MB": " 12 "}), want: with(func(c *Config) { c.Log.MaxSizeMB = 12 })},
		{name: "int: не число", env: withBase(map[string]string{"LOG_MAX_SIZE_MB": "abc"}), want: defaults, warnings: []Warning{{Var: "LOG_MAX_SIZE_MB", Value: "abc", Default: 20}}},
		{name: "int: ноль", env: withBase(map[string]string{"LOG_MAX_SIZE_MB": "0"}), want: defaults, warnings: []Warning{{Var: "LOG_MAX_SIZE_MB", Value: "0", Default: 20}}},
		{name: "int: отрицательное", env: withBase(map[string]string{"LOG_MAX_AGE_DAYS": "-3"}), want: defaults, warnings: []Warning{{Var: "LOG_MAX_AGE_DAYS", Value: "-3", Default: 14}}},

		// int64. ID каналов отрицательные и не помещаются в int32.
		{name: "int64: ID канала", env: withBase(map[string]string{"CHANNEL_ID": "-1001234567890"}), want: with(func(c *Config) { c.ChannelID = -1001234567890 })},
		{name: "int64: пусто", env: map[string]string{"BOT_TOKEN": "123:abc"}, want: with(func(c *Config) { c.ChannelID = 0 }), wantErr: ErrNoChannel},
		{
			name: "int64: не число", env: withBase(map[string]string{"CHANNEL_ID": "-100abc"}), want: with(func(c *Config) { c.ChannelID = 0 }),
			warnings: []Warning{{Var: "CHANNEL_ID", Value: "-100abc", Default: int64(0)}}, wantErr: ErrNoChannel,
		},

		// bool.
		{name: "bool: пусто", env: withBase(map[string]string{"LOG_COMPRESS": ""}), want: defaults},
		{name: "bool: false", env: withBase(map[string]string{"LOG_COMPRESS": "false"}), want: with(func(c *Config) { c.Log.Compress = false })},
		{name: "bool: 0", env: withBase(map[string]string{"BOT_AUTH_ENABLED": "0"}), want: with(func(c *Config) { c.AuthEnabled = false })},
		// Все bool-переменные по умолчанию true, поэтому TRUE проверяется
		// отсутствием предупреждения: некорректное значение дало бы Warning.
		{name: "bool: TRUE", env: withBase(map[string]string{"BOT_AUTH_ENABLED": " TRUE "}), want: defaults},
		{name: "bool: не bool", env: withBase(map[string]string{"BOT_AUTH_ENABLED": "да"}), want: defaults, warnings: []Warning{{Var: "BOT_AUTH_ENABLED", Value: "да", Default: true}}},

		// LOG_LEVEL: без учёта регистра и пробелов.
		{name: "LOG_LEVEL: пусто", env: withBase(map[string]string{"LOG_LEVEL": ""}), want: defaults},
		{name: "LOG_LEVEL: info", env: withBase(map[string]string{"LOG_LEVEL": "info"}), want: defaults},
		{name: "LOG_LEVEL: INFO", env: withBase(map[string]string{"LOG_LEVEL": "INFO"}), want: defaults},
		{name: "LOG_LEVEL: debug", env: withBase(map[string]string{"LOG_LEVEL": "debug"}), want: with(func(c *Config) { c.Log.Debug = true })},
		{name: "LOG_LEVEL: Debug с пробелами", env: withBase(map[string]string{"LOG_LEVEL": " Debug "}), want: with(func(c *Config) { c.Log.Debug = true })},
		{name: "LOG_LEVEL: trace", env: withBase(map[string]string{"LOG_LEVEL": "trace"}), want: defaults, warnings: []Warning{{Var: "LOG_LEVEL", Value: "trace", Default: "info"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, warnings, err := Load(func(key string) string { return tt.env[key] })

			if tt.wantErr == nil && err != nil {
				t.Fatalf("получена ошибка %v, ожидалось без ошибки", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("получена ошибка %v, ожидалось %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("получено %+v, ожидалось %+v", got, tt.want)
			}
			if !slices.Equal(warnings, tt.warnings) {
				t.Errorf("получены предупреждения %#v, ожидалось %#v", warnings, tt.warnings)
			}
		})
	}
}

func TestErrorTexts(t *testing.T) {
	for err, want := range map[error]string{
		ErrNoToken:   "не задан BOT_TOKEN",
		ErrNoChannel: "не задан или некорректен CHANNEL_ID",
	} {
		if got := err.Error(); got != want {
			t.Errorf("получено %q, ожидалось %q", got, want)
		}
	}
}
