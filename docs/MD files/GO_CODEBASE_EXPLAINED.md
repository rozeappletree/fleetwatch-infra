# 🛠️ FleetTrack India: Go Codebase Breakdown

This document provides a detailed explanation of the Go (Golang) implementation for the FleetTrack India backend. The code is organized into a shared package (`fleetbridge`) and three specialized microservices (cmd binaries).

---

## 📦 Core Package (`hslservices/`)

This package contains the shared logic, data structures, and client initializations used by all services.

### 1. `event.go`

- **Purpose:** Defines the "Language of the Fleet."
- **Key Logic:**
  - `TruckEvent` Struct: Maps the compact JSON telemetry from the Flutter app to a Go object.
  - `GetEventHash()`: Generates a unique MD5 hash for each trip. This hash is used as a **Redis Key Prefix**, ensuring that data from different trips never mixes.
  - `EventHolder`: A wrapper used for unmarshaling incoming MQTT payloads.

### 2. `mqttClient.go`

- **Purpose:** Manages the connection to the Mosquitto broker.
- **Key Logic:**
  - `MsgBroker`: Implements a "Staging Channel" (buffered Go channel). This acts as a high-speed intake valve.
  - `messageHandler`: A non-blocking function that takes messages from the network and pushes them into the staging channel.
  - `InitMQTTClient`: Configures auto-reconnect logic and subscriptions to `trucks/#`.

### 3. `redisClient.go`

- **Purpose:** Centralized Redis Stack connection management.
- **Key Logic:**
  - `InitRedisClient`: Handles authentication and connection pooling. It also performs a "Ping" on startup to ensure the database is ready before the service starts processing data.

### 4. `errors.go`

- **Purpose:** Custom error handling.
- **Key Logic:** Defines `MQTTValidationError`, used when a payload is technically valid JSON but contains unusable data (like zero GPS coordinates).

---

## 🚀 Microservices (`hslservices/cmd/`)

### 1. Ingestion Worker (`cmd/mqtt/main.go`)

- **Role:** The "Heart" of the system. It moves data from the network to the database.
- **How it works:**
  - Spawns **10 concurrent workers** (goroutines).
  - Uses **Redis Pipelines** to perform four operations atomically:
    1.  `PUBLISH`: Sends the position to the live dashboard.
    2.  `XADD`: Writes the event to a stream for permanent storage in PostGIS.
    3.  `TS.ADD (Speed)`: Saves speed for the history graph.
    4.  `TS.ADD (Geohash)`: Saves the location for the history map trail.

### 2. Locations API (`cmd/locations/main.go`)

- **Role:** Powers the real-time Dashboard.
- **How it works:**
  - **WebSocket Handler:** Maintains live connections with browser clients.
  - **Pub/Sub Fanout:** Listens to Redis for new vehicle positions and "fans them out" to all connected browsers instantly.
  - **History Handler:** Provides a REST endpoint (`/histlocations/`) that queries Redis TimeSeries to reconstruct a vehicle's path for the last hour.

### 3. Tiles API (`cmd/tiles/main.go`)

- **Role:** Serves map data to the dashboard.
- **How it works:**
  - A lightweight static file server optimized for **Mapbox Vector Tiles (.pbf)**.
  - It reads pre-generated map layers (like India district boundaries) from the disk and streams them to the browser for rendering.

---

## ⚡ Performance Summary

- **Concurrency:** Heavy use of `goroutines` and `channels` ensures the system never blocks while waiting for the network.
- **Memory Efficiency:** Most services run in "Distroless" containers, consuming less than 10MB of RAM at idle.
- **Spatial Intelligence:** Uses `geohash` encoding to store GPS coordinates as integers inside Redis, making historical lookups extremely fast.
