# syntax=docker/dockerfile:1.7
# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /tiles-api ./cmd/tiles

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="Fleet Tiles API"
LABEL org.opencontainers.image.description="Serves pre-generated .pbf map tiles."

COPY --from=builder /tiles-api /tiles-api

USER nonroot:nonroot
EXPOSE 2151
ENTRYPOINT ["/tiles-api"]
