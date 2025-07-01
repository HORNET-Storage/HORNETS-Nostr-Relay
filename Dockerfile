# === Stage 1: Build React Panel ===
FROM node:18-alpine AS panel-builder

WORKDIR /panel

COPY HORNETS-Relay-Panel/package.json HORNETS-Relay-Panel/yarn.lock ./
RUN yarn install --frozen-lockfile

COPY HORNETS-Relay-Panel/ ./
RUN yarn build

# === Stage 2: Build Go Relay ===
FROM golang:1.23-alpine AS relay-builder

WORKDIR /app
RUN apk add --no-cache git gcc musl-dev

COPY HORNETS-Nostr-Relay/go.mod HORNETS-Nostr-Relay/go.sum ./
RUN go mod download

COPY HORNETS-Nostr-Relay/ ./
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o relay ./services/server/port

# === Stage 3: Final Minimal Runtime Image ===
FROM alpine:latest

RUN addgroup -S appgroup && adduser -S appuser -G appgroup && apk add --no-cache ca-certificates chromium

WORKDIR /app
COPY --from=relay-builder /app/relay ./
COPY --from=panel-builder /panel/build ./web

RUN mkdir -p /app/data /app/temp /app/statistics && chown -R appuser:appgroup /app

USER appuser

CMD ["./relay"]
