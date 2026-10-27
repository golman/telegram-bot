// Package logging настраивает логи бота: slog в формате key=value со временем
// в UTC, вывод в stdout и в файл с ротацией, адаптер логов telego.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/mymmrac/telego"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"

	"github.com/golman/telegram-bot/internal/config"
)

// logLevel — уровень логов бота, его выставляет LOG_LEVEL.
var logLevel = new(slog.LevelVar)

// NewHandler возвращает обработчик в формате логов бота: текст «key=value»,
// время в UTC. Тесты перехватывают логи тем же обработчиком.
func NewHandler(w io.Writer, level slog.Leveler) slog.Handler {
	return slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				a.Value = slog.TimeValue(a.Value.Time().UTC())
			}
			return a
		},
	})
}

// Bootstrap направляет логи в stdout в формате бота, чтобы записи до вызова
// Setup (и при его ошибке) не терялись и не меняли формат. Уровень общий
// с Setup: до него — info.
func Bootstrap(stdout io.Writer) {
	slog.SetDefault(slog.New(NewHandler(stdout, logLevel)))
}

// Setup направляет логи в stdout и в файл с ротацией, выставляет уровень
// из cfg.Debug и пишет запись «логирование настроено». При ошибке вывод
// по умолчанию не меняется.
func Setup(cfg config.Log, stdout io.Writer) error {
	if err := os.MkdirAll(filepath.Dir(cfg.FilePath), 0o755); err != nil {
		return fmt.Errorf("создание каталога логов: %w", err)
	}

	rotatingLog := &lumberjack.Logger{
		Filename:   cfg.FilePath,
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
	}

	slog.SetDefault(slog.New(NewHandler(io.MultiWriter(stdout, rotatingLog), logLevel)))
	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}
	logLevel.Set(level)
	slog.Info("логирование настроено", "file", cfg.FilePath, "log_level", logLevel.Level(),
		"max_size_mb", cfg.MaxSizeMB, "max_backups", cfg.MaxBackups, "max_age_days", cfg.MaxAgeDays, "compress", cfg.Compress)
	return nil
}

// telegoLogger направляет логи telego в slog, то есть в stdout и в файл
// вместе с остальными логами бота. Токен в сообщениях заменяется
// на BOT_TOKEN. Debug-записи (URL запросов, их тела и ответы API с текстами
// объявлений и данными пользователей) пишутся только при LOG_LEVEL=debug.
type telegoLogger struct {
	debug    bool
	replacer *strings.Replacer
}

func newTelegoLogger(token string, debug bool) *telegoLogger {
	return &telegoLogger{debug: debug, replacer: strings.NewReplacer(token, telego.DefaultLoggerTokenReplacement)}
}

func (l *telegoLogger) Debugf(format string, args ...any) {
	if l.debug {
		slog.Debug(l.text(format, args...))
	}
}

func (l *telegoLogger) Errorf(format string, args ...any) {
	slog.Error(l.text(format, args...))
}

func (l *telegoLogger) text(format string, args ...any) string {
	return "telego: " + l.replacer.Replace(fmt.Sprintf(format, args...))
}

// BotOptions возвращает настройки логирования клиента telego: его логи идут
// в slog с маскировкой token. В режиме debug telego дополнительно пишет
// параметры запросов.
func BotOptions(token string, debug bool) []telego.BotOption {
	opts := []telego.BotOption{telego.WithLogger(newTelegoLogger(token, debug))}
	if debug {
		opts = append(opts, telego.WithDebugMode())
	}
	return opts
}
