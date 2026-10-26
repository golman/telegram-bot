# syntax=docker/dockerfile:1

# Образы закреплены по digest: тег могут перевесить на другой образ.
# Dependabot обновляет digest вместе с тегом.

# Сборка. Версия Go — последняя стабильная; минимальная из go.mod
# проверяется в CI.
FROM golang:1.27.3-trixie@sha256:fbb7ae40b62ff8afe31662df570d39b4f5052ac32b90dae8e60d7657a20a74f4 AS build

WORKDIR /src
# Не скачивать другую версию Go, даже если go.mod попросит.
ENV GOTOOLCHAIN=local CGO_ENABLED=0

# Модули скачиваются отдельным слоем, чтобы правка кода их не перекачивала.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY *.go ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/telegram-bot .

# Каталог логов с владельцем nonroot (65532): в distroless нет shell,
# создать его там нельзя. Именованный том при первом подключении
# получает этого владельца.
RUN mkdir -p /out/logs

# Запуск: только бинарник, без shell и пакетного менеджера, не от root.
# В образе есть корневые сертификаты для HTTPS к Bot API.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99

WORKDIR /app
COPY --from=build /out/telegram-bot /app/telegram-bot
COPY --from=build --chown=65532:65532 /out/logs /app/logs

ENV LOG_FILE_PATH=/app/logs/telegram-bot.log
VOLUME ["/app/logs"]

USER 65532:65532
ENTRYPOINT ["/app/telegram-bot"]
