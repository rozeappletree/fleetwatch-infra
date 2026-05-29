# syntax=docker/dockerfile:1.7
# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Install build deps (CGO disabled — pure Go binary, smaller image)
RUN apk add --no-cache git

# Cache dependency layer before copying source
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /fleet-worker ./cmd/mqtt

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="Fleet Worker (MQTT → Redis)"
LABEL org.opencontainers.image.description="Subscribes to Mosquitto, writes TruckEvent to Redis."

COPY --from=builder /fleet-worker /fleet-worker

USER nonroot:nonroot
ENTRYPOINT ["/fleet-worker"]
