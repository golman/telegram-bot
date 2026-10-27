// Package testutil содержит хелперы, общие для тестов всех пакетов.
package testutil

import (
	"bytes"
	"io"
	"log"
	"log/slog"
	"testing"
)

// CaptureLog перенаправляет логи slog в буфер до конца теста. Формат тот же,
// что в проде (key=value, время в UTC), уровень debug. После теста
// восстанавливаются slog.Default и вывод пакета log.
func CaptureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	// slog.SetDefault перенаправляет и пакет log; его вывод восстанавливается отдельно.
	prevOut, prevFlags := log.Writer(), log.Flags()
	slog.SetDefault(slog.New(newHandler(&buf, slog.LevelDebug)))
	t.Cleanup(func() {
		slog.SetDefault(prev)
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return &buf
}

// newHandler повторяет newLogHandler из package main: тот пока недоступен
// из других пакетов. Дублирование временное — когда обработчик переедет
// в internal/logging, здесь будет logging.NewHandler.
func newHandler(w io.Writer, level slog.Leveler) slog.Handler {
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
