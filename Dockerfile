# syntax=docker/dockerfile:1

## ---- build stage ----
FROM golang:1.26 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -buildvcs=false -trimpath -ldflags="-s -w" \
    -o /out/yt-music-bot .

## ---- runtime stage ----
FROM debian:stable-slim AS runtime

# ffmpeg (with libmp3lame) for MP3 encoding + thumbnail resize;
# ca-certificates for TLS to YouTube / Telegram.
RUN apt-get update \
    && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -u 1000 -d /app app
WORKDIR /app
COPY --from=builder /out/yt-music-bot /app/yt-music-bot
USER app

ENTRYPOINT ["/app/yt-music-bot"]
