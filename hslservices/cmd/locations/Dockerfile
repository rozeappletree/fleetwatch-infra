# syntax=docker/dockerfile:1.7
# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /build

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /locations-api ./cmd/locations

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="Fleet Locations API"
LABEL org.opencontainers.image.description="WebSocket fan-out + historical trip query API."

COPY --from=builder /locations-api /locations-api

USER nonroot:nonroot
EXPOSE 2152
ENTRYPOINT ["/locations-api"]
