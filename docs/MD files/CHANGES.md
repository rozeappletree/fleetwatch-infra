# CHANGES — Helsinki HSL → FleetTrack India

Migration log documenting every change made when pivoting from the original
Helsinki Regional Transit (HSL) codebase to the FleetTrack India production platform.

---

## 1. Project Identity

| Item | Old (HSL) | New (FleetTrack India) |
|------|-----------|------------------------|
| Project name | Helsinki Regional Transit Tracking | FleetTrack India |
| Go package name | `hsldatabridge` | `fleetbridge` |
| Go module path | `github.com/dmw2151/hsldatabridge` | `github.com/dmw2151/fleetbridge` |
| Data source | HSL public MQTT feed (`mqtt.hsl.fi:8883`) | Our own Mosquitto broker (internal LAN) |
| Map tiles | CARTO raster tiles (proprietary, API key) | OpenStreetMap via OpenLayers 8 (free, no key) |
| Geofence data | Helsinki postal code shapefiles (HSL-specific) | GADM 4.1 India state & district boundaries |
| Design doc | `Helsinki_Transit_System_Design.md` | `FleetTrack_India_System_Design.md` |

---

## 2. Go Backend (`hslservices/`)

### 2.1 Go Toolchain & Dependencies

| Package | Old Version | New Version | Notes |
|---------|-------------|-------------|-------|
| Go toolchain | `1.16` | `1.22` | |
| `go-redis` | `go-redis/redis/v8 v8.8.2` | `redis/go-redis/v9 v9.7.3` | Module path changed |
| `paho.mqtt.golang` | `v1.3.4` | `v1.5.0` | |
| `logrus` | `v1.8.1` | `v1.9.4` | |
| `gorilla/mux` | `v1.8.0` | `v1.8.1` | |
| `gorilla/websocket` | `v1.4.2` | `v1.5.3` | |
| `golang.org/x/sync` | `v0.0.0-20210220...` | `v0.10.0` | |
| `ffjson` | `v0.0.0-20190930...` | **removed** | Replaced by `encoding/json` |
| `opentelemetry` (otel) | `v0.20.0` (3 pkgs) | **removed** | Was never used |

### 2.2 `hslservices/event.go` — Data Model

**Old:** `Event` struct with HSL GTFS journey fields.
```go
// OLD
type Event struct {
    Jrn       int     // journey number
    Line      int     // line number
    Oday      string  // operating day
    RouteID   string  // GTFS route ID
    Lat, Lng  float64
    Spd, Dl   float32 // speed + delay (HSL-specific)
    // ...
}
```

**New:** `TruckEvent` struct with compact JSON keys for 2G/3G efficiency.
```go
// NEW
type TruckEvent struct {
    VehicleID string  `json:"vid"` // registration plate
    DriverID  string  `json:"did"` // driver phone/ID
    TripID    string  `json:"tid"` // UUID per trip
    Lat       float64 `json:"lat"`
    Lng       float64 `json:"lng"`
    Speed     float32 `json:"spd"`
    Bearing   float32 `json:"brg"` // NEW — heading
    Accuracy  float32 `json:"acc"` // NEW — GPS accuracy metres
    Timestamp int64   `json:"ts"`
    Battery   int     `json:"bat"` // NEW — device battery %
    Source    string  `json:"src"` // NEW — "mobile"|"obd"|"sim"
}
```

**Trip key changed:** `MD5(jrn:routeID:oday)` → `MD5(VehicleID:TripID)`

**Removed:** `event_ffjson.go` — 900-line auto-generated ffjson file deleted entirely.

### 2.3 `hslservices/mqttClient.go`

| | Old | New |
|--|-----|-----|
| Broker | `mqtt.hsl.fi:8883` (external, TLS) | `mosquitto:1883` (internal, plain TCP) |
| Protocol | TLS with CA cert | Plain TCP |
| Topic | `/hfp/v2/journey/+/vp/bus/#` | `trucks/#` |
| Deserialisation | `ffjson` | `encoding/json` |
| Package | `hsldatabridge` | `fleetbridge` |

### 2.4 `hslservices/cmd/mqtt/main.go`

| | Old | New |
|--|-----|-----|
| Event type | `hsl.Event` | `fleet.TruckEvent` via `fleet.EventHolder` |
| Trip key label | `journey=<id>` | `trip=<id>` |
| Redis stream fields | `rt jid lat lng time spd acc dl` | `vid did tid key lat lng spd brg acc ts bat src` |
| TimeSeries label | `journey=<hash>` | `trip=<hash>` |
| Delay field (`dl`) | written to stream | **removed** (not applicable to trucks) |
| Redis client | `go-redis/v8` | `redis/go-redis/v9` |

### 2.5 `hslservices/cmd/locations/main.go`

| | Old | New |
|--|-----|-----|
| Package import | `hsldatabridge` | `fleetbridge` |
| Redis client | `go-redis/v8` + `client.Context()` | `go-redis/v9` + `context.Background()` |
| History endpoint body | `{ jrn, route, oday }` | `{ "vid": "...", "tid": "..." }` |
| TS.MRANGE filter | `FILTER journey=<id>` | `FILTER trip=<id>` |
| Response type | `[]hsl.Event` | `[]fleet.TruckEvent` |
| Speed field | `.Spd` | `.Speed` |
| Health response | `"Good Morning, Helsinki!"` | `"Fleet Tracker India — OK"` |

### 2.6 `hslservices/cmd/tiles/main.go`

| | Old | New |
|--|-----|-----|
| Health response | `"Good Morning, Helsinki!"` | `"Fleet Tracker India — Tiles OK"` |

### 2.7 `hslservices/redisClient.go`

| | Old | New |
|--|-----|-----|
| Package | `hsldatabridge` | `fleetbridge` |
| Redis import | `go-redis/redis/v8` | `redis/go-redis/v9` |

---

## 3. Docker Infrastructure

### 3.1 Docker Images

| Service | Old Image | New Image | Reason |
|---------|-----------|-----------|--------|
| `redis` | `dmw2151/redismods` (unofficial, archived) | `redis/redis-stack-server:7.4.0-v3` | Official Redis Ltd image |
| `postgis` | `mdillon/postgis` (archived 2021) | `postgis/postgis:16-3.5` | Official, actively maintained |
| `frontend` | `node:15.14.0-alpine3.10` + parcel | `nginx:1.27-alpine` | No npm build needed |
| `mqtt` worker | `golang:1.16-alpine` → `alpine:latest` | `golang:1.22-alpine` → `distroless:nonroot` | Go 1.22, ~3 MB runtime |
| `locations_api` | `golang:1.16-alpine` → `alpine:latest` | `golang:1.22-alpine` → `distroless:nonroot` | Same |
| `tiles_api` | `golang:1.16-alpine` → `alpine:latest` | `golang:1.22-alpine` → `distroless:nonroot` | Same |

### 3.2 New Service Added

| Service | Image | Purpose |
|---------|-------|---------|
| `mosquitto` | `eclipse-mosquitto:2` | Our own MQTT broker — driver phones publish here |

### 3.3 Dockerfile Changes (Go services)

**Old pattern:**
- Builder: `golang:1.16-alpine` with `CGO_ENABLED=1`
- Runtime: `alpine:latest` with `mosquitto-libs` apk packages
- Build command: `cd ./cmd/mqtt && go build -o mqttconnector`

**New pattern:**
- Builder: `golang:1.22-alpine` with `CGO_ENABLED=0` (pure Go)
- Runtime: `gcr.io/distroless/static-debian12:nonroot` (~3 MB, no shell)
- Build command: `go build -ldflags="-s -w" -o /fleet-worker ./cmd/mqtt`
- Proper layer caching: `COPY go.mod go.sum` before source copy
- Runs as `nonroot:nonroot` user

### 3.4 `docker-compose.yml`

| Change | Old | New |
|--------|-----|-----|
| Mosquitto config path | `./demo_server/mosquitto/mosquitto.conf` | `./mosquitto/mosquitto.conf` |
| Frontend port mapping | `8080:1234` | `8080:8080` |
| Frontend volume | `./frontend/dist/:/fleet_dashboard/dist` | **removed** (nginx serves static) |
| Redis image | `dmw2151/redismods` | `redis/redis-stack-server:7.4.0-v3` |
| PostGIS image | `mdillon/postgis` | `postgis/postgis:16-3.5` |

---

## 4. Frontend (`frontend/`)

| Item | Old | New |
|------|-----|-----|
| Framework | Vanilla JS + OpenLayers 6 + Parcel bundler | Pure HTML + OpenLayers 8 via CDN |
| Map base tiles | CARTO raster (API key required) | OpenStreetMap (free, no key) |
| Build step | `npm install && npm run build` | None — static file |
| Server | `npm start` (parcel dev server) | `nginx:1.27-alpine` |
| Vehicle data | HSL GTFS bus positions | `TruckEvent` JSON via WebSocket |
| Status bar | `"Helsinki (HH:MM) ☀️ — N HSL vehicles on the road"` | Fleet-specific status with truck count |
| `index.js` | 500+ lines of HSL-specific OL3 map code | Stub comment only (active code is in `index.html`) |
| `Dockerfile` | `FROM node:15.14.0-alpine3.10` | `FROM nginx:1.27-alpine` |
| New file | — | `nginx.conf` — proxy WS, tiles, history API |

---

## 5. Tilegen (`tilegen/`)

### `tilegen/tippecanoe/get_static_data.sh`

| | Old | New |
|--|-----|-----|
| Data source | HSL GTFS zip (`infopalvelut.storage.hsldev.com`) | GADM 4.1 India boundaries (`geodata.ucdavis.edu`) |
| PostGIS table | `helsinki_places` (postcode areas) | `india_states` + `india_districts` |
| Tile layers | `stops`, `routes` (GTFS) | `statistics` (district-level speed choropleth) |

### `tilegen/tippecanoe/tilegen.sh`

| | Old | New |
|--|-----|-----|
| SQL join table | `helsinki_places hp` | `india_districts d` |
| Output columns | `nimi`, `spd`, `dl` (delay) | `district`, `state`, `spd` |
| Materialized view refresh | `REFRESH MATERIALIZED VIEW event_log` | `REFRESH MATERIALIZED VIEW CONCURRENTLY event_log` |

---

## 6. Redis (`redis/`)

| Item | Old | New |
|------|-----|-----|
| Base image | `redislabs/redismod:latest` (archived) | `redis/redis-stack-server:7.4.0-v3` (official) |
| Python version | Python 2 (via `curl pip bootstrap`) | Python 3 (system packages) |
| `requirements.txt` | Unpinned: `psycopg2-binary`, `redis`, `numpy`, `gears-cli`, `click` | Pinned: `psycopg2-binary==2.9.10`, `redis==5.2.1`, `numpy==2.2.2`, `click==8.1.8` |
| `gears-cli` | included | **removed** (incompatible with Redis Stack 7.x) |
| User | root | `redis` (non-root) |

---

## 7. Environment Files (`envs/`)

### `envs/mqtt_connector.env`

| Variable | Old | New |
|----------|-----|-----|
| `MQTT_BROKER` | `mqtt.hsl.fi` | `mosquitto` |
| `MQTT_PORT` | `8883` (TLS) | `1883` (plain TCP) |
| `MQTT_TOPIC` | `/hfp/v2/journey/+/vp/bus/#` | `trucks/#` |

---

## 8. New Files Added

| File | Purpose |
|------|---------|
| `mosquitto/mosquitto.conf` | Mosquitto broker config (promoted from `demo_server/`) |
| `frontend/nginx.conf` | nginx reverse proxy config for WS + tile routes |
| `docs/fleettrack_architecture.png` | Architecture diagram image |
| `FleetTrack_India_System_Design.md` | Full system design doc (replaced old Helsinki doc) |
| `CHANGES.md` | This file |
| `driver_app/` | Flutter Android driver app (entire new directory) |

---

## 9. Files Removed

| File / Directory | Reason |
|-----------------|--------|
| `demo_server/` | Replaced by the full Docker stack |
| `hslservices/event_ffjson.go` | Deleted — `ffjson` replaced by `encoding/json` |
| `Helsinki_Transit_System_Design.md` | Renamed to `FleetTrack_India_System_Design.md` |
| `frontend/package-lock.json` | No npm build step anymore |

---

## 10. Removed HSL-Specific References (Strings)

| Location | Old String | Replaced With |
|----------|-----------|---------------|
| `marketplace.json` | `"Helsinki Transit Tracking"` | `"FleetTrack India — Real-Time Truck Tracking"` |
| `marketplace.json` | `"Helsinki Transit Feeds using Redis"` | Fleet tracking description |
| `readme.md` | Full HSL architecture description | FleetTrack India architecture |
| `locations/main.go` | `"Good Morning, Helsinki!"` | `"Fleet Tracker India — OK"` |
| `tiles/main.go` | `"Good Morning, Helsinki!"` | `"Fleet Tracker India — Tiles OK"` |
| `get_static_data.sh` | `## Download Helsinki GTFS Data` | India GADM download |
| `tilegen.sh` | `from helsinki_places hp` | `from india_districts d` |
| `docker-compose.yml` | `dmw2151/redismods`, `mdillon/postgis` | Official images |
