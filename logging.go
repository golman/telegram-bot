package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	tgbotapi "github.com/mymmrac/telego"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"

	"github.com/golman/telegram-bot/internal/config"
)

// logLevel — уровень логов бота, его выставляет LOG_LEVEL.
var logLevel = new(slog.LevelVar)

// newLogHandler — формат логов бота: текст «key=value», время в UTC.
// Тесты перехватывают логи тем же обработчиком.
func newLogHandler(w io.Writer, level slog.Leveler) slog.Handler {
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

// initLogging читает конфиг, настраивает по нему логи и пишет в них
// предупреждения конфига — уже после открытия файла, чтобы они попали и туда.
// Возвращает ошибку настройки логов или ошибку config.Load (нет BOT_TOKEN
// или CHANNEL_ID); во втором случае логи всё равно настроены.
func initLogging(getenv func(string) string) (config.Config, error) {
	cfg, warnings, loadErr := config.Load(getenv)
	setupErr := setupLogging(cfg.Log)
	// При ошибке настройки файла предупреждения уходят хотя бы в stdout.
	for _, w := range warnings {
		slog.Warn("некорректная переменная окружения, используется значение по умолчанию",
			"var", w.Var, "value", w.Value, "default", w.Default)
	}
	if setupErr != nil {
		return cfg, setupErr
	}
	return cfg, loadErr
}

// setupLogging направляет логи в stdout и в файл с ротацией и выставляет
// уровень из cfg.Debug.
func setupLogging(cfg config.Log) error {
	// До открытия файла логи идут в stdout в том же формате.
	slog.SetDefault(slog.New(newLogHandler(os.Stdout, logLevel)))

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

	slog.SetDefault(slog.New(newLogHandler(io.MultiWriter(os.Stdout, rotatingLog), logLevel)))
	if cfg.Debug {
		logLevel.Set(slog.LevelDebug)
	}
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
	return &telegoLogger{debug: debug, replacer: strings.NewReplacer(token, tgbotapi.DefaultLoggerTokenReplacement)}
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

// botOptions — настройки логирования клиента telego. В режиме debug telego
// дополнительно пишет параметры запросов.
func botOptions(token string, debug bool) []tgbotapi.BotOption {
	opts := []tgbotapi.BotOption{tgbotapi.WithLogger(newTelegoLogger(token, debug))}
	if debug {
		opts = append(opts, tgbotapi.WithDebugMode())
	}
	return opts
}
