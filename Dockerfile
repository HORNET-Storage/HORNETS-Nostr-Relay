FROM node:18-alpine AS panel-builder

# Build the React panel
WORKDIR /panel
COPY HORNETS-Relay-Panel/package.json HORNETS-Relay-Panel/yarn.lock ./
RUN yarn install --frozen-lockfile

COPY HORNETS-Relay-Panel/ ./
RUN yarn build

FROM golang:1.23-alpine AS relay-builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

# Copy go mod files first for better caching
COPY HORNETS-Nostr-Relay/go.mod HORNETS-Nostr-Relay/go.sum ./
RUN go mod download

# Copy source code
COPY HORNETS-Nostr-Relay/ .

# Build the relay
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o relay ./services/server/port

FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates chromium

WORKDIR /app

# Copy binary and web assets
COPY --from=relay-builder /app/relay .
COPY --from=panel-builder /panel/build ./web

# Create necessary directories
RUN mkdir -p /app/data /app/temp /app/statistics

# Expose ports (libp2p: 9000, websocket: 9001, web panel: 9002)
EXPOSE 9000 9001 9002

CMD ["./relay"]
