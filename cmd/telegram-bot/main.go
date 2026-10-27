// Команда telegram-bot запускает бота: читает конфиг из окружения,
// настраивает логи, подключается к Bot API и работает до SIGINT/SIGTERM.
package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	tgbotapi "github.com/mymmrac/telego"

	"github.com/golman/telegram-bot/internal/bot"
	"github.com/golman/telegram-bot/internal/config"
	"github.com/golman/telegram-bot/internal/logging"
	"github.com/golman/telegram-bot/internal/tgapi"
)

// fatal логирует ошибку запуска и завершает процесс с кодом 1.
func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

// initLogging читает конфиг, настраивает по нему логи и пишет в них
// предупреждения конфига — уже после открытия файла, чтобы они попали и туда.
// Возвращает ошибку настройки логов или ошибку config.Load (нет BOT_TOKEN
// или CHANNEL_ID); во втором случае логи всё равно настроены.
func initLogging(getenv func(string) string, stdout io.Writer) (config.Config, error) {
	cfg, warnings, loadErr := config.Load(getenv)
	setupErr := logging.Setup(cfg.Log, stdout)
	// При ошибке настройки файла предупреждения уходят хотя бы в stdout
	// (туда логи направил logging.Bootstrap).
	for _, w := range warnings {
		slog.Warn("некорректная переменная окружения, используется значение по умолчанию",
			"var", w.Var, "value", w.Value, "default", w.Default)
	}
	if setupErr != nil {
		return cfg, setupErr
	}
	return cfg, loadErr
}

func main() {
	logging.Bootstrap(os.Stdout)
	cfg, err := initLogging(os.Getenv, os.Stdout)
	if errors.Is(err, config.ErrNoToken) || errors.Is(err, config.ErrNoChannel) {
		fatal(err.Error())
	}
	if err != nil {
		fatal("не удалось настроить логирование", "err", err)
	}

	opts := append(logging.BotOptions(cfg.Token, cfg.Log.Debug), tgbotapi.WithAPICaller(tgapi.NewCaller()))
	client, err := tgbotapi.NewBot(cfg.Token, opts...)
	if err != nil {
		fatal("не удалось создать клиент Bot API", "err", err)
	}

	// SIGINT/SIGTERM отменяют ctx: telego прекращает long polling и закрывает
	// канал обновлений, после чего Run дообрабатывает буфер и выходит.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		// Повторный сигнал снова завершает процесс сразу, если остановка зависла.
		stop()
	}()

	updates, err := client.UpdatesViaLongPolling(ctx, &tgbotapi.GetUpdatesParams{Timeout: tgapi.LongPollTimeoutSeconds})
	if err != nil {
		fatal("не удалось запустить long polling", "err", err)
	}

	if err := bot.New(client, cfg).Run(ctx, updates); err != nil {
		stop()
		fatal("бот остановился с ошибкой", "err", err)
	}
}
