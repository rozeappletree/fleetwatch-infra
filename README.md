# FleetWatch Infrastructure & Backend

Welcome to the **FleetWatch Infra** repository. This repository contains the backend services, message brokers, caching layers, and database infrastructure for the FleetWatch real-time geospatial tracking platform. 

The architecture is designed for high-throughput vehicle telemetry, processing MQTT messages, persisting spatial data in PostGIS, and caching real-time states in Redis, which is then served to the frontend dashboard.

## System Architecture

The infrastructure consists of several microservices orchestrated via Docker Compose:

1. **MQTT Broker (EMQX)**: Handles high-frequency, low-latency telemetry pings from the driver mobile app or simulation pipelines.
2. **Redis**: Acts as the real-time cache layer for active vehicle coordinates, trips, and state management.
3. **PostGIS (PostgreSQL)**: The primary spatial database. Stores permanent geospatial infrastructure (ports, berths, highways, railways) and historical telemetry data.
4. **Golang Backend (`hslservices`)**: 
   - **Worker**: Subscribes to the MQTT topics, validates telemetry, updates Redis, and inserts historical records into PostGIS.
   - **Locations API**: A REST/WebSocket API that serves the real-time vehicle data from Redis to the React/Leaflet frontend.
5. **Tilegen**: A Python service/sidecar for processing and serving vector/raster map tiles and mocked geospatial data generation.

## Directory Structure

```
.
├── docker-compose.yml   # Main orchestration file for all services
├── docs/                # Geospatial maps (QGIS projects) and diagrams
├── driver_app/          # Mobile application source code (Flutter)
├── envs/                # Environment variable templates
├── hslservices/         # Golang API and MQTT Worker services
├── mock_geojson/        # Scripts for generating mock telemetry and port boundaries
├── emqx/                # MQTT broker configuration
├── postgis/             # SQL schema definitions and init scripts for PostGIS
├── redis/               # Redis configuration
└── tilegen/             # Python-based tile generation and map serving utilities
```

## Setup & Deployment

### Prerequisites

- Docker and Docker Compose installed.
- (Optional) Go 1.21+ for local backend development.
- (Optional) Flutter SDK for local mobile app development.

### Running the Infrastructure

To spin up the entire infrastructure locally:

```bash
# Start all background services (Redis, PostGIS, MQTT, Golang API, Worker)
docker compose up -d

# Check the logs of the Go API to ensure it's connected
docker compose logs -f locations_api

# Open the EMQX dashboard in local development
# http://localhost:18083
```

### Database Initialization

The PostGIS database initializes automatically using the SQL files found in `/postgis/`. 
- `01_schema.sql` creates the foundational tracking schemas.
- `02_port_schema.sql` sets up the spatial tables for custom port maps (berths, boundaries).

### Sending Test Telemetry

You can inject mock telemetry data into the MQTT broker to test the data pipeline without running the driver app:

```bash
docker exec -it fleetwatch-emqx emqx_ctl pub -t "trucks/truck_01" -m '{"vehicle_id": "truck_01", "lat": 17.7416, "lng": 83.3559}'

# Or use any external MQTT client against localhost:1883.
# Example:
mosquitto_pub -h localhost -p 1883 -t "trucks/truck_01" -m '{"vehicle_id": "truck_01", "lat": 17.7416, "lng": 83.3559}'
```

## License
MIT License
