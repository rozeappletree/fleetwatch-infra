# FleetTrack India — Backend Infrastructure

Real-time GPS tracking and geofencing backend for Visakhapatnam Port bulk-cargo logistics.

```
[Driver Phone]
  Flutter App → MQTT QoS 1
        ↓
  [EMQX Broker]  port 1883
        ↓
  [Go Worker]  (hslservices/cmd/mqtt)
        ↓
  Redis Pipeline:
    PUBLISH → WebSocket fan-out
    XADD    → PostGIS write-behind
    TS.ADD  → Speed + position time-series
    GEOADD  → fleet:live spatial index
    HSET    → truck:{vid} state (5-min TTL)
    Geofence checks (in-memory, zero DB hits)
        ↓
  [Locations API]  port 2152
    WebSocket  → live map dashboard
    REST       → fleet geo queries + port map
```

---

## Quick Start

```bash
docker compose up -d
```

| Service | Port | Description |
|---|---|---|
| **EMQX** | 1883 / 8083 / 18083 | MQTT broker (TCP / WS / dashboard) |
| **Redis Stack** | 6379 | TimeSeries, Streams, GEO, Pub/Sub |
| **Go Worker** | — | MQTT → Redis bridge + geofence engine |
| **Locations API** | 2152 | WebSocket fan-out + REST endpoints |
| **PostGIS** | 5433 | GPS event log + port spatial data |
| **Martin** | 3001 | MVT vector tile server (OSM layers) |

---

## Project Structure

```
.
├── hslservices/              # Go backend (package: fleetbridge)
│   ├── event.go              # TruckEvent payload model
│   ├── mqttClient.go         # EMQX subscriber (auto-reconnect, re-subscribe)
│   ├── redisClient.go        # Redis Stack client (go-redis v9)
│   ├── errors.go
│   └── cmd/
│       ├── mqtt/
│       │   ├── main.go       # MQTT → Redis pipeline (6-step TxPipeline)
│       │   └── geofence.go   # In-memory geofence engine (PostGIS + GEOSEARCH)
│       └── locations/
│           ├── main.go       # WebSocket fan-out + router
│           ├── geo.go        # GET /fleet/current/ · GET /fleet/nearby/
│           └── trips.go      # Port map API (berths, yards, corridors, gates)
│
├── emqx/                     # EMQX broker config
│   ├── emqx.conf
│   └── acl.conf
│
├── redis/                    # Redis Stack 7.4 image + write-behind script
├── postgis/                  # PostGIS init SQL (port schema, statistics)
├── martin/                   # Martin MVT tile server config
├── mock_geojson/             # Test zone data (berths, gates, corridors, yards)
├── envs/                     # Environment variable files
├── tools/                    # Dev utilities
├── cargo/                    # App clients (gitignored — developed separately)
│   ├── context/              # Flutter driver app (Android foreground service)
│   └── frontend/             # React live map dashboard
└── docker-compose.yml
```

---

## API Endpoints

### Locations API (`:2152`)

| Method | Path | Description |
|---|---|---|
| `WS` | `/locations/` | Live truck positions (Redis Pub/Sub fan-out) |
| `POST` | `/histlocations/` | Trip history from Redis TimeSeries |
| `GET` | `/fleet/current/` | All online trucks (HGETALL pipeline) |
| `GET` | `/fleet/nearby/?lat=&lng=&radius=&unit=` | Trucks within radius (GEOSEARCH) |
| `GET` | `/api/berths/` | Active port berths with geometry |
| `GET` | `/api/yards/` | Active yards (`?owner_type=client\|competitor\|vpa`) |
| `GET` | `/api/corridors/` | Active route corridors with GeoJSON polygon |
| `GET` | `/api/gates/` | All port gates |
| `POST` | `/api/trips/assign` | Assign truck to berth → returns corridor |

### Martin Tile Server (`:3001`)

| Path | Description |
|---|---|
| `/catalog` | Discover all tile sources |
| `/{fn_name}/{z}/{x}/{y}` | MVT tiles (roads, railways, water, places, POI) |

---

## Redis Key Design

| Key | Type | TTL | Description |
|---|---|---|---|
| `fleet:live` | GEO sorted set | none | Spatial index, all known trucks |
| `truck:{vid}` | Hash | 5 min | Live state — lat, lng, spd, brg, ts, bat, tid, did |
| `positions:{key}:speed` | TimeSeries | 60 s | Speed (km/h) rolling window |
| `positions:{key}:gh` | TimeSeries | 60 s | Geohash int rolling window |
| `positions:{key}:speed:agg` | TimeSeries | 2 h | 15 s aggregated speed |
| `positions:{key}:gh:agg` | TimeSeries | 2 h | 15 s aggregated position |
| `events` | Stream | — | Write-behind to PostGIS |
| `currentLocationsPS` | Pub/Sub | — | WebSocket fan-out channel |
| `tripKeys` | Set | — | Seen trip hashes (idempotent TS.CREATE) |

---

## GPS Payload Format

```json
{
  "vid": "AP 30 Y 1828",
  "did": "9505683966",
  "tid": "uuid-trip-id",
  "lat": 17.7137197,
  "lng": 83.1691558,
  "spd": 0.30,
  "brg": 0.0,
  "acc": 5.10,
  "ts":  1778132507,
  "bat": 82,
  "src": "mobile"
}
```

---

## Environment Variables

### `envs/mqtt_connector.env`
```env
MQTT_TOPIC=trucks/#
MQTT_BROKER=emqx
MQTT_PORT=1883
MQTT_N_WORKERS=10
```

### `envs/redis.env`
```env
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_DB=0
```

---

## Tech Stack

| Layer | Technology |
|---|---|
| MQTT broker | EMQX 5.6.1 |
| Go worker | Go 1.22, paho.mqtt v1.5, go-redis v9 |
| Cache & streams | Redis Stack 7.4 (TimeSeries, GEO, Pub/Sub, Streams) |
| Spatial DB | PostGIS 16-3.5 |
| Geofence engine | planar.RingContains (orb) + Redis GEOSEARCH |
| Vector tiles | Martin 0.18 + PostGIS MVT functions |
| Driver app | Flutter 3, Android Foreground Service (cargo/context/) |
| Dashboard | React + MapLibre GL JS (cargo/frontend/) |
