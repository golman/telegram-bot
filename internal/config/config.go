// Package config читает настройки бота из переменных окружения.
//
// Load — чистая функция: она ничего не логирует, а о некорректных значениях
// сообщает списком Warning, чтобы main записал их, когда логи уже настроены.
package config

import (
	"errors"
	"strconv"
	"strings"
)

// Значения по умолчанию.
const (
	defaultLogFilePath   = "logs/telegram-bot.log"
	defaultLogMaxSizeMB  = 20
	defaultLogMaxBackups = 5
	defaultLogMaxAgeDays = 14
	defaultLogCompress   = true
	defaultLogLevel      = "info"
	defaultAuthEnabled   = true
)

// Ошибки обязательных переменных. Их текст main пишет в лог перед выходом.
var (
	ErrNoToken   = errors.New("не задан BOT_TOKEN")
	ErrNoChannel = errors.New("не задан или некорректен CHANNEL_ID")
)

// Log — настройки логов: файл с ротацией (lumberjack) и уровень.
type Log struct {
	FilePath   string // LOG_FILE_PATH
	MaxSizeMB  int    // LOG_MAX_SIZE_MB
	MaxBackups int    // LOG_MAX_BACKUPS
	MaxAgeDays int    // LOG_MAX_AGE_DAYS
	Compress   bool   // LOG_COMPRESS
	Debug      bool   // LOG_LEVEL=debug
}

// Config — все настройки бота.
type Config struct {
	Token       string // BOT_TOKEN
	ChannelID   int64  // CHANNEL_ID, целевой канал
	AuthEnabled bool   // BOT_AUTH_ENABLED: принимать объявления только от участников канала
	Log         Log
}

// Warning сообщает, что переменная Var задана некорректно (значение Value)
// и вместо неё взято значение по умолчанию Default.
type Warning struct {
	Var, Value string
	Default    any
}

// Load читает настройки через getenv (в проде — os.Getenv). Значения
// обрезаются по краям; пустое значение молча заменяется умолчанием,
// некорректное — умолчанием с Warning. Числа логов должны быть больше нуля,
// LOG_LEVEL — info или debug без учёта регистра.
//
// Если не задан BOT_TOKEN или CHANNEL_ID, возвращается ErrNoToken или
// ErrNoChannel, но Config (в том числе Log) и Warning всё равно заполнены,
// чтобы main мог настроить логи и записать ошибку в файл.
func Load(getenv func(string) string) (Config, []Warning, error) {
	l := loader{getenv: getenv}
	cfg := Config{
		Log: Log{
			FilePath:   l.str("LOG_FILE_PATH", defaultLogFilePath),
			MaxSizeMB:  l.positiveInt("LOG_MAX_SIZE_MB", defaultLogMaxSizeMB),
			MaxBackups: l.positiveInt("LOG_MAX_BACKUPS", defaultLogMaxBackups),
			MaxAgeDays: l.positiveInt("LOG_MAX_AGE_DAYS", defaultLogMaxAgeDays),
			Compress:   l.boolean("LOG_COMPRESS", defaultLogCompress),
			Debug:      l.debugLevel("LOG_LEVEL"),
		},
		Token:       l.str("BOT_TOKEN", ""),
		ChannelID:   l.int64("CHANNEL_ID", 0),
		AuthEnabled: l.boolean("BOT_AUTH_ENABLED", defaultAuthEnabled),
	}

	var err error
	switch {
	case cfg.Token == "":
		err = ErrNoToken
	case cfg.ChannelID == 0:
		err = ErrNoChannel
	}
	return cfg, l.warnings, err
}

// loader разбирает переменные и копит предупреждения.
type loader struct {
	getenv   func(string) string
	warnings []Warning
}

func (l *loader) raw(key string) string {
	return strings.TrimSpace(l.getenv(key))
}

func (l *loader) warn(key, raw string, fallback any) {
	l.warnings = append(l.warnings, Warning{Var: key, Value: raw, Default: fallback})
}

func (l *loader) str(key, fallback string) string {
	if raw := l.raw(key); raw != "" {
		return raw
	}
	return fallback
}

func (l *loader) positiveInt(key string, fallback int) int {
	raw := l.raw(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		l.warn(key, raw, fallback)
		return fallback
	}
	return value
}

func (l *loader) int64(key string, fallback int64) int64 {
	raw := l.raw(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		l.warn(key, raw, fallback)
		return fallback
	}
	return value
}

func (l *loader) boolean(key string, fallback bool) bool {
	raw := l.raw(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		l.warn(key, raw, fallback)
		return fallback
	}
	return value
}

// debugLevel разбирает уровень логов и возвращает true для debug.
func (l *loader) debugLevel(key string) bool {
	raw := l.raw(key)
	switch strings.ToLower(raw) {
	case "", "info":
		return false
	case "debug":
		return true
	default:
		l.warn(key, raw, defaultLogLevel)
		return false
	}
}
