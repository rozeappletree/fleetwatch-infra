# syntax=docker/dockerfile:1.7
# Fleet Tracker — Dashboard (Vite + React + MapLibre)

# ── Stage 1: Build Vite React App ───────────────────────────────────────────
FROM node:20-alpine AS builder

WORKDIR /app
COPY package*.json ./
RUN npm install

COPY . .
RUN npm run build

# ── Stage 2: Serve with Nginx ───────────────────────────────────────────────
FROM nginx:1.27-alpine

LABEL org.opencontainers.image.title="Fleet Dashboard"
LABEL org.opencontainers.image.description="MapLibre live tracking dashboard."

# Remove default nginx page
RUN rm -rf /usr/share/nginx/html/*

# Copy built assets from builder
COPY --from=builder /app/dist /usr/share/nginx/html/

# Copy nginx config: serve on :8080, proxy /locations/, /histlocations/ and /tiles/
COPY ./nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 8080
CMD ["sh", "-c", "mkdir -p /usr/share/nginx/html/cdn && printf '{\"version\":1,\"mqtt\":{\"host\":\"%s\",\"port\":%s,\"protocol\":\"tcp\"}}\n' \"${PUBLIC_MQTT_HOST:-localhost}\" \"${PUBLIC_MQTT_PORT:-1883}\" > /usr/share/nginx/html/cdn/fleet-config.json && nginx -g 'daemon off;'"]
