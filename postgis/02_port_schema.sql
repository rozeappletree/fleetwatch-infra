-- =============================================================================
-- port_schema.sql
-- Vizag Port custom map layer definitions + spatial indexes
-- Run after ogr2ogr loads the GeoJSON layers.
-- All geometry stored in EPSG:4326.
-- =============================================================================

-- Enable PostGIS if not already enabled
CREATE EXTENSION IF NOT EXISTS postgis;

-- =============================================================================
-- DIGITISED LAYERS (loaded by ogr2ogr from QGIS export)
-- ogr2ogr creates these tables automatically; the statements below ensure they
-- exist even if the GeoJSON export hasn't been run yet, and add extra indexes.
-- =============================================================================

-- Port boundary (outer perimeter fence)
CREATE TABLE IF NOT EXISTS port_boundary (
    id              SERIAL PRIMARY KEY,
    name            TEXT,
    wkb_geometry    GEOMETRY(Polygon, 4326)
);

-- Berths / dock areas
CREATE TABLE IF NOT EXISTS berths (
    id              SERIAL PRIMARY KEY,
    berth_no        TEXT,
    name            TEXT,
    capacity        INTEGER,
    wkb_geometry    GEOMETRY(Polygon, 4326)
);

-- Client cargo yards
CREATE TABLE IF NOT EXISTS client_yards (
    id              SERIAL PRIMARY KEY,
    yard_id         TEXT,
    name            TEXT,
    material_type   TEXT,
    owner           TEXT DEFAULT 'client',
    wkb_geometry    GEOMETRY(Polygon, 4326)
);

-- Competitor cargo yards
CREATE TABLE IF NOT EXISTS competitor_yards (
    id              SERIAL PRIMARY KEY,
    yard_id         TEXT,
    name            TEXT,
    owner           TEXT,   -- competitor company name
    wkb_geometry    GEOMETRY(Polygon, 4326)
);

-- Truck roads (internal port road network)
CREATE TABLE IF NOT EXISTS truck_roads (
    id              SERIAL PRIMARY KEY,
    name            TEXT,
    road_type       TEXT CHECK (road_type IN ('main', 'access', 'internal')),
    surface         TEXT,
    wkb_geometry    GEOMETRY(LineString, 4326)
);

-- Entry / exit gates
CREATE TABLE IF NOT EXISTS gates (
    id              SERIAL PRIMARY KEY,
    name            TEXT,
    gate_type       TEXT CHECK (gate_type IN ('entry', 'exit', 'both')),
    active          BOOLEAN DEFAULT TRUE,
    wkb_geometry    GEOMETRY(Point, 4326)
);

-- Weighbridge locations
CREATE TABLE IF NOT EXISTS weighbridges (
    id              SERIAL PRIMARY KEY,
    name            TEXT,
    operator        TEXT,
    active          BOOLEAN DEFAULT TRUE,
    wkb_geometry    GEOMETRY(Point, 4326)
);

-- No-go / restricted zones
CREATE TABLE IF NOT EXISTS no_go_zones (
    id              SERIAL PRIMARY KEY,
    name            TEXT,
    reason          TEXT,
    wkb_geometry    GEOMETRY(Polygon, 4326)
);

-- =============================================================================
-- ROUTE CORRIDORS
-- Generated from truck_roads via ST_Buffer (reproject to UTM 44N for meters,
-- then back to 4326). Populated by the load_geojson.sh script after ogr2ogr.
-- =============================================================================

CREATE TABLE IF NOT EXISTS route_corridors (
    id              SERIAL PRIMARY KEY,
    name            TEXT        NOT NULL,
    source_name     TEXT,
    dest_name       TEXT,
    centerline      GEOMETRY(LineString, 4326),
    corridor        GEOMETRY(Polygon, 4326),
    active          BOOLEAN     DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- Populate corridors from truck_roads (100 m buffer, UTM 44N → 4326)
-- Run this once after ogr2ogr loads truck_roads.
INSERT INTO route_corridors (name, source_name, dest_name, centerline, corridor)
SELECT
    COALESCE(name, 'Road-' || id::text) AS name,
    NULL                                AS source_name,
    NULL                                AS dest_name,
    wkb_geometry                        AS centerline,
    ST_Transform(
        ST_Buffer(
            ST_Transform(wkb_geometry, 32644),  -- UTM zone 44N (Vizag)
            100                                  -- 100 metre buffer
        ),
        4326
    )                                   AS corridor
FROM truck_roads
ON CONFLICT DO NOTHING;

-- =============================================================================
-- SPATIAL INDEXES (GIST) — critical for Go worker point-in-polygon checks
-- =============================================================================

CREATE INDEX IF NOT EXISTS idx_port_boundary_geom
    ON port_boundary USING GIST(wkb_geometry);

CREATE INDEX IF NOT EXISTS idx_berths_geom
    ON berths USING GIST(wkb_geometry);

CREATE INDEX IF NOT EXISTS idx_client_yards_geom
    ON client_yards USING GIST(wkb_geometry);

CREATE INDEX IF NOT EXISTS idx_competitor_yards_geom
    ON competitor_yards USING GIST(wkb_geometry);

CREATE INDEX IF NOT EXISTS idx_truck_roads_geom
    ON truck_roads USING GIST(wkb_geometry);

CREATE INDEX IF NOT EXISTS idx_gates_geom
    ON gates USING GIST(wkb_geometry);

CREATE INDEX IF NOT EXISTS idx_weighbridges_geom
    ON weighbridges USING GIST(wkb_geometry);

CREATE INDEX IF NOT EXISTS idx_no_go_zones_geom
    ON no_go_zones USING GIST(wkb_geometry);

CREATE INDEX IF NOT EXISTS idx_route_corridors_corridor
    ON route_corridors USING GIST(corridor);

-- =============================================================================
-- CONVENIENCE VIEW — active corridors + point source for the Martin tile server
-- =============================================================================

CREATE OR REPLACE VIEW active_corridors AS
SELECT id, name, source_name, dest_name, corridor AS geom, created_at
FROM route_corridors
WHERE active = TRUE;
