package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tgbotapi "github.com/mymmrac/telego"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultLogFilePath   = "logs/telegram-bot.log"
	defaultLogMaxSizeMB  = 20
	defaultLogMaxBackups = 5
	defaultLogMaxAgeDays = 14
	defaultLogCompress   = true
)

type fileLogConfig struct {
	filePath   string
	maxSizeMB  int
	maxBackups int
	maxAgeDays int
	compress   bool
}

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

// setupLogging направляет логи в stdout и в файл с ротацией и выставляет
// уровень из LOG_LEVEL. Возвращает true, если включён уровень debug.
func setupLogging() (bool, error) {
	// До открытия файла логи идут в stdout в том же формате: туда попадут
	// предупреждения о некорректных LOG_* при разборе конфига.
	slog.SetDefault(slog.New(newLogHandler(os.Stdout, logLevel)))
	cfg := loadLogConfigFromEnv()

	if err := os.MkdirAll(filepath.Dir(cfg.filePath), 0o755); err != nil {
		return false, fmt.Errorf("создание каталога логов: %w", err)
	}

	rotatingLog := &lumberjack.Logger{
		Filename:   cfg.filePath,
		MaxSize:    cfg.maxSizeMB,
		MaxBackups: cfg.maxBackups,
		MaxAge:     cfg.maxAgeDays,
		Compress:   cfg.compress,
	}

	slog.SetDefault(slog.New(newLogHandler(io.MultiWriter(os.Stdout, rotatingLog), logLevel)))
	// LOG_LEVEL разбирается после настройки вывода, чтобы предупреждение
	// о некорректном значении попало в файл.
	debug := parseLogLevel(getEnvOrDefault("LOG_LEVEL", "info"))
	if debug {
		logLevel.Set(slog.LevelDebug)
	}
	slog.Info("логирование настроено", "file", cfg.filePath, "log_level", logLevel.Level(),
		"max_size_mb", cfg.maxSizeMB, "max_backups", cfg.maxBackups, "max_age_days", cfg.maxAgeDays, "compress", cfg.compress)

	return debug, nil
}

// warnBadEnv сообщает, что переменная окружения задана некорректно.
func warnBadEnv(key, raw string, fallback any) {
	slog.Warn("некорректная переменная окружения, используется значение по умолчанию",
		"var", key, "value", raw, "default", fallback)
}

func loadLogConfigFromEnv() fileLogConfig {
	return fileLogConfig{
		filePath:   getEnvOrDefault("LOG_FILE_PATH", defaultLogFilePath),
		maxSizeMB:  getEnvIntOrDefault("LOG_MAX_SIZE_MB", defaultLogMaxSizeMB),
		maxBackups: getEnvIntOrDefault("LOG_MAX_BACKUPS", defaultLogMaxBackups),
		maxAgeDays: getEnvIntOrDefault("LOG_MAX_AGE_DAYS", defaultLogMaxAgeDays),
		compress:   getEnvBoolOrDefault("LOG_COMPRESS", defaultLogCompress),
	}
}

func getEnvOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvIntOrDefault(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		warnBadEnv(key, raw, fallback)
		return fallback
	}
	return value
}

func getEnvBoolOrDefault(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		warnBadEnv(key, raw, fallback)
		return fallback
	}
	return value
}

func getEnvInt64OrDefault(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		warnBadEnv(key, raw, fallback)
		return fallback
	}
	return value
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

// parseLogLevel разбирает LOG_LEVEL и возвращает true для уровня debug.
// Поддерживаются info (по умолчанию) и debug; другое значение логируется
// и заменяется на info.
func parseLogLevel(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "info":
		return false
	case "debug":
		return true
	default:
		slog.Warn("некорректный LOG_LEVEL, используется info", "value", raw)
		return false
	}
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
