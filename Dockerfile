# syntax=docker/dockerfile:1

# Образы закреплены по digest: тег могут перевесить на другой образ.
# Dependabot обновляет digest вместе с тегом.

# Сборка. Версия Go — последняя стабильная; минимальная из go.mod
# проверяется в CI.
FROM golang:1.27.3-trixie@sha256:af1bfc2614ebb1dbd31af514b78740fd678850e76498762b425ecee4c5c6ea80 AS build

WORKDIR /src
# Не скачивать другую версию Go, даже если go.mod попросит.
ENV GOTOOLCHAIN=local CGO_ENABLED=0

# Модули скачиваются отдельным слоем, чтобы правка кода их не перекачивала.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/telegram-bot ./cmd/telegram-bot

# Каталог логов с владельцем nonroot (65532): в distroless нет shell,
# создать его там нельзя. Именованный том при первом подключении
# получает этого владельца.
RUN mkdir -p /out/logs

# Запуск: только бинарник, без shell и пакетного менеджера, не от root.
# В образе есть корневые сертификаты для HTTPS к Bot API.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329

WORKDIR /app
COPY --from=build /out/telegram-bot /app/telegram-bot
COPY --from=build --chown=65532:65532 /out/logs /app/logs

ENV LOG_FILE_PATH=/app/logs/telegram-bot.log
VOLUME ["/app/logs"]

USER 65532:65532
ENTRYPOINT ["/app/telegram-bot"]
