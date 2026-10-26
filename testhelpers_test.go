package main

import (
	"bytes"
	"log"
	"testing"
	"time"
)

// captureLog перенаправляет стандартный логгер в буфер до конца теста.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return &buf
}

// fakeClock — управляемые часы для тикера альбомов: время двигает тест.
type fakeClock struct{ t time.Time }

// useFakeClock подставляет боту управляемые часы.
func useFakeClock(b *VBBot) *fakeClock {
	c := &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	b.now = func() time.Time { return c.t }
	return c
}

// wait сдвигает время на d и делает шаг тикера.
func (c *fakeClock) wait(b *VBBot, d time.Duration) {
	c.t = c.t.Add(d)
	b.tick()
}
