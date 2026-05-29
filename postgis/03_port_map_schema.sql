-- =============================================================================
-- 03_port_map_schema.sql
-- FleetTrack India — VPA port_map schema + seed data
-- Auto-run by PostGIS container on first boot (docker-entrypoint-initdb.d).
--
-- Schema: port_map (isolated from public/statistics)
-- All geometry: EPSG:4326  |  Buffers done in EPSG:32644 (UTM 44N)
-- =============================================================================

-- Enable PostGIS (idempotent)
CREATE EXTENSION IF NOT EXISTS postgis;

-- Dedicated schema
CREATE SCHEMA IF NOT EXISTS port_map;

-- =============================================================================
-- 1. PORT BOUNDARY
-- =============================================================================
CREATE TABLE IF NOT EXISTS port_map.port_boundary (
    id          SERIAL PRIMARY KEY,
    name        TEXT DEFAULT 'VPA Boundary',
    area_acres  NUMERIC,
    geom        GEOMETRY(Polygon, 4326)
);

-- =============================================================================
-- 2. VPA ZONES
-- =============================================================================
CREATE TABLE IF NOT EXISTS port_map.vpa_zones (
    id          SERIAL PRIMARY KEY,
    zone_no     TEXT NOT NULL,
    area_acres  NUMERIC,
    land_use    TEXT,
    geom        GEOMETRY(Polygon, 4326)
);

-- =============================================================================
-- 3. BERTHS
-- =============================================================================
CREATE TABLE IF NOT EXISTS port_map.berths (
    id           SERIAL PRIMARY KEY,
    berth_code   TEXT NOT NULL UNIQUE,
    berth_name   TEXT,
    quay_type    TEXT CHECK (quay_type IN (
                     'east_quay','west_quay','ore_berth',
                     'container','oil_wharf','other')),
    operator     TEXT,
    capacity_mt  NUMERIC,
    harbour      TEXT CHECK (harbour IN ('inner','outer')),
    active       BOOLEAN DEFAULT true,
    geom         GEOMETRY(Polygon, 4326)
);

-- =============================================================================
-- 4. YARDS
-- =============================================================================
CREATE TABLE IF NOT EXISTS port_map.yards (
    id           SERIAL PRIMARY KEY,
    yard_code    TEXT NOT NULL UNIQUE,
    yard_name    TEXT,
    owner        TEXT,
    owner_type   TEXT CHECK (owner_type IN ('client','competitor','vpa','public')),
    material     TEXT,
    area_acres   NUMERIC,
    active       BOOLEAN DEFAULT true,
    geom         GEOMETRY(Polygon, 4326)
);

-- =============================================================================
-- 5. PORT ROADS
-- =============================================================================
CREATE TABLE IF NOT EXISTS port_map.port_roads (
    id           SERIAL PRIMARY KEY,
    road_code    TEXT NOT NULL,
    road_name    TEXT,
    row_meters   INTEGER,
    road_type    TEXT CHECK (road_type IN ('main','internal','access')),
    direction    TEXT CHECK (direction IN ('one_way','two_way')),
    surface      TEXT DEFAULT 'paved',
    geom         GEOMETRY(LineString, 4326)
);

-- =============================================================================
-- 6. GATES
-- =============================================================================
CREATE TABLE IF NOT EXISTS port_map.gates (
    id           SERIAL PRIMARY KEY,
    gate_code    TEXT NOT NULL UNIQUE,
    gate_name    TEXT,
    gate_type    TEXT CHECK (gate_type IN ('entry','exit','both')),
    rfid         BOOLEAN DEFAULT false,
    active       BOOLEAN DEFAULT true,
    notes        TEXT,
    geom         GEOMETRY(Point, 4326)
);

-- =============================================================================
-- 7. WEIGHBRIDGES
-- =============================================================================
CREATE TABLE IF NOT EXISTS port_map.weighbridges (
    id           SERIAL PRIMARY KEY,
    wb_code      TEXT NOT NULL UNIQUE,
    wb_name      TEXT,
    operator     TEXT,
    active       BOOLEAN DEFAULT true,
    geom         GEOMETRY(Point, 4326)
);

-- =============================================================================
-- 8. ROUTE CORRIDORS
-- =============================================================================
CREATE TABLE IF NOT EXISTS port_map.route_corridors (
    id              SERIAL PRIMARY KEY,
    corridor_code   TEXT NOT NULL UNIQUE,
    corridor_name   TEXT,
    berth_id        INTEGER REFERENCES port_map.berths(id),
    yard_id         INTEGER REFERENCES port_map.yards(id),
    road_sequence   TEXT[],
    entry_gate      TEXT,
    exit_gate       TEXT,
    buffer_meters   INTEGER DEFAULT 80,
    centerline      GEOMETRY(LineString, 4326),
    corridor        GEOMETRY(Polygon, 4326),
    active          BOOLEAN DEFAULT true,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- =============================================================================
-- 9. NO-GO ZONES
-- =============================================================================
CREATE TABLE IF NOT EXISTS port_map.no_go_zones (
    id           SERIAL PRIMARY KEY,
    zone_name    TEXT,
    reason       TEXT CHECK (reason IN (
                     'defence_area','railway_crossing',
                     'residential','restricted','water_body')),
    geom         GEOMETRY(Polygon, 4326)
);

-- =============================================================================
-- 10. TRIPS
-- =============================================================================
CREATE TABLE IF NOT EXISTS port_map.trips (
    id           SERIAL PRIMARY KEY,
    trip_id      TEXT NOT NULL UNIQUE,
    vehicle_id   TEXT NOT NULL,
    berth_id     INTEGER REFERENCES port_map.berths(id),
    yard_id      INTEGER REFERENCES port_map.yards(id),
    corridor_id  INTEGER REFERENCES port_map.route_corridors(id),
    berth_code   TEXT,
    yard_code    TEXT,
    shift        TEXT CHECK (shift IN ('day','night','general')),
    status       TEXT DEFAULT 'assigned'
                      CHECK (status IN ('assigned','active','completed','cancelled')),
    assigned_at  TIMESTAMPTZ DEFAULT NOW(),
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

-- =============================================================================
-- SPATIAL INDEXES
-- =============================================================================
CREATE INDEX IF NOT EXISTS idx_pm_port_boundary   ON port_map.port_boundary    USING GIST(geom);
CREATE INDEX IF NOT EXISTS idx_pm_vpa_zones        ON port_map.vpa_zones        USING GIST(geom);
CREATE INDEX IF NOT EXISTS idx_pm_berths           ON port_map.berths            USING GIST(geom);
CREATE INDEX IF NOT EXISTS idx_pm_yards            ON port_map.yards             USING GIST(geom);
CREATE INDEX IF NOT EXISTS idx_pm_port_roads       ON port_map.port_roads        USING GIST(geom);
CREATE INDEX IF NOT EXISTS idx_pm_gates            ON port_map.gates             USING GIST(geom);
CREATE INDEX IF NOT EXISTS idx_pm_weighbridges     ON port_map.weighbridges      USING GIST(geom);
CREATE INDEX IF NOT EXISTS idx_pm_corridors        ON port_map.route_corridors   USING GIST(corridor);
CREATE INDEX IF NOT EXISTS idx_pm_no_go            ON port_map.no_go_zones       USING GIST(geom);
CREATE INDEX IF NOT EXISTS idx_pm_trips_vehicle    ON port_map.trips             (vehicle_id);
CREATE INDEX IF NOT EXISTS idx_pm_trips_trip_id    ON port_map.trips             (trip_id);

-- =============================================================================
-- CONVENIENCE VIEWS
-- =============================================================================
CREATE OR REPLACE VIEW port_map.active_corridors AS
SELECT id, corridor_code, corridor_name, berth_id, yard_id,
       road_sequence, entry_gate, exit_gate, buffer_meters,
       centerline, corridor AS geom, created_at
FROM port_map.route_corridors
WHERE active = true;

CREATE OR REPLACE VIEW port_map.competitor_geofences AS
SELECT id, yard_code, yard_name, owner, material, geom
FROM port_map.yards
WHERE owner_type = 'competitor' AND active = true;

-- =============================================================================
-- SEED DATA — geometry = NULL, digitized later in QGIS
-- =============================================================================

-- Berths
INSERT INTO port_map.berths (berth_code,berth_name,quay_type,operator,capacity_mt,harbour)
VALUES
    ('EQ1',   'East Quay 1',          'east_quay','VPA',          8.6,   'inner'),
    ('EQ1A',  'East Quay 1A',         'east_quay','VPA',          NULL,  'inner'),
    ('EQ3',   'East Quay 3',          'east_quay','VPA',          6.0,   'inner'),
    ('EQ4',   'East Quay 4',          'east_quay','VPA',          NULL,  'inner'),
    ('EQ6',   'East Quay 6',          'east_quay','Everson Marine',3.0,  'inner'),
    ('EQ7',   'East Quay 7',          'east_quay','VMTPL',        3.0,   'inner'),
    ('EQ8',   'East Quay 8',          'east_quay','VSPL',         14.2,  'inner'),
    ('EQ9',   'East Quay 9',          'east_quay','VSPL',         NULL,  'inner'),
    ('EQ10',  'East Quay 10',         'east_quay','AVR Infra',    4.7,   'inner'),
    ('WQ1',   'West Quay 1',          'west_quay','VPA',          9.58,  'inner'),
    ('WQ2',   'West Quay 2',          'west_quay','VPA',          21.54, 'inner'),
    ('WQ3',   'West Quay 3',          'west_quay','VPA',          NULL,  'inner'),
    ('WQ4',   'West Quay 4',          'west_quay','VPA',          NULL,  'inner'),
    ('WQ5',   'West Quay 5',          'west_quay','NALCO',        NULL,  'inner'),
    ('WQ6',   'West Quay 6',          'west_quay','ICTPL',        6.3,   'inner'),
    ('WQ7',   'West Quay 7',          'west_quay','Bothra/AMNS',  12.6,  'inner'),
    ('WQ8',   'West Quay 8',          'west_quay','Bothra/AMNS',  NULL,  'inner'),
    ('OB1',   'Ore Berth 1 (EVTL)',   'ore_berth','EVTL',         20.1,  'outer'),
    ('OB2',   'Ore Berth 2 (EVTL)',   'ore_berth','EVTL',         NULL,  'outer'),
    ('VGCB',  'VGCB 2L DWT',          'ore_berth','Vedanta',      12.4,  'outer'),
    ('VCTPL1','Container Terminal 1', 'container','JM Baxi',      39.2,  'outer'),
    ('VCTPL2','Container Terminal 2', 'container','JM Baxi',      NULL,  'outer'),
    ('SPM',   'Single Point Mooring', 'oil_wharf','HPCL',         NULL,  'outer'),
    ('LPG',   'LPG Berth',           'oil_wharf','VPA',          15.0,  'inner'),
    ('OR1',   'Oil Wharf 1',          'oil_wharf','VPA',          NULL,  'inner'),
    ('OR2',   'Oil Wharf 2',          'oil_wharf','VPA',          15.44, 'inner'),
    ('OR3',   'Oil Wharf 3',          'oil_wharf','VPA',          NULL,  'inner'),
    ('FB',    'Fertilizer Berth',     'other',    'Coromandel',   1.3,   'inner'),
    ('OSTT',  'Offshore Tank Terminal','other',   'VPA',          NULL,  'outer'),
    ('REWQ1', 'WQ Return End',         'other',   'VPA',          NULL,  'inner'),
    ('GCB',   'Green Channel Berth',  'other',    'VPA',          NULL,  'inner')
ON CONFLICT (berth_code) DO NOTHING;

-- Gates
INSERT INTO port_map.gates (gate_code,gate_name,gate_type,rfid)
VALUES
    ('G1',   'RFID Gate 1',           'both',  true),
    ('G2',   'RFID Gate 2',           'both',  true),
    ('G3',   'RFID Gate 3',           'both',  true),
    ('G4',   'RFID Gate 4',           'both',  true),
    ('G5',   'RFID Gate 5',           'both',  true),
    ('G6',   'RFID Gate 6',           'both',  true),
    ('G7',   'RFID Gate 7',           'both',  true),
    ('STP',  'STP Pond Gate',         'both',  false),
    ('BRAMP','B Ramp Gate',           'exit',  false),
    ('SBC',  'SBC Junction Gate',     'both',  false),
    ('AMB',  'Dr. Ambedkar Junction', 'entry', false),
    ('CONV', 'Convent Junction',      'both',  false),
    ('DOCK', 'Dock Main Gate',        'both',  false),
    ('HIQ',  'HIQ Main Gate',         'both',  false)
ON CONFLICT (gate_code) DO NOTHING;

-- Port Roads
INSERT INTO port_map.port_roads (road_code,row_meters,road_type,direction)
VALUES
    ('VP1', 12,'internal','one_way'),('VP2', 12,'internal','one_way'),
    ('VP3', 15,'main',    'one_way'),('VP4', 12,'internal','one_way'),
    ('VP5', 12,'access',  'two_way'),('VP6', 12,'internal','one_way'),
    ('VP7', 15,'main',    'two_way'),('VP8', 12,'internal','one_way'),
    ('VP9', 12,'internal','one_way'),('VP10',15,'main',    'one_way'),
    ('VP11',15,'main',    'two_way'),('VP12',18,'main',    'one_way'),
    ('VP13',18,'main',    'two_way'),('VP14',12,'access',  'two_way'),
    ('VP15',12,'access',  'two_way'),('VP16',12,'internal','two_way'),
    ('VP17',12,'access',  'two_way'),('VP19',30,'main',    'one_way'),
    ('VP20',18,'main',    'one_way'),('VP21',15,'internal','one_way'),
    ('VP22',45,'main',    'two_way'),('VP23',18,'main',    'two_way'),
    ('VP24',15,'internal','one_way'),('VP25',15,'internal','one_way'),
    ('VP26',18,'main',    'two_way');

-- Yards
INSERT INTO port_map.yards (yard_code,yard_name,owner,owner_type,material)
VALUES
    ('EQ25_SY',  'EQ 2-5 Stack Yard',      'VPA',              'vpa',        'bulk'),
    ('WQ14_SY',  'WQ 1-4 Stack Yard',       'VPA',              'vpa',        'bulk'),
    ('WQRE_SY',  'WQ RE Stack Yard',         'VPA',              'vpa',        'bulk'),
    ('SIL_SY',   'SIL Yard',                'SIL',              'vpa',        'bulk'),
    ('OPEN_SY',  'Open Stack Area',          'VPA',              'vpa',        'bulk'),
    ('TM_PLOT1', 'TM Plot Zone 5',           'VPA',              'vpa',        'logistics'),
    ('TM_PLOT2', 'TM Plot Zone 6',           'VPA',              'vpa',        'logistics'),
    ('EQ89_SY',  'EQ 8-9 VSPL Yard',        'VSPL',             'competitor', 'bulk'),
    ('EVTL_SY',  'EVTL Stack Yard',          'EVTL',             'competitor', 'iron_ore'),
    ('VGCB_SY',  'VGCB Stack Yard',          'Vedanta',          'competitor', 'coal'),
    ('VCTPL1_SY','VCTPL-I Stack Yard',       'JM Baxi',          'competitor', 'containers'),
    ('VCTPL2_SY','VCTPL-II Stack Yard',      'JM Baxi',          'competitor', 'containers'),
    ('NALCO_SY', 'NALCO Yard',               'NALCO',            'competitor', 'alumina'),
    ('CIL_SY',   'CIL (EQ6) Yard',           'CIL',              'competitor', 'bulk'),
    ('BOTHRA_SY','Bothra Yard',              'Bothra Shipping',  'competitor', 'bulk'),
    ('WQ6_SY',   'WQ-6 Stack Yard',          'ICTPL',            'competitor', 'bulk'),
    ('WQ78_SY',  'WQ 7-8 Stack Yard',        'Bothra/AMNS',      'competitor', 'steel'),
    ('FB_SY',    'Fertilizer Berth Yard',    'Coromandel',       'competitor', 'fertilizer'),
    ('GARUDA_SY','Garuda Coal Yard',          'Garuda Coal',      'competitor', 'coal'),
    ('ORISSA_SY','Orissa Stevedores Yard',   'Orissa Stevedores','competitor', 'bulk'),
    ('CARBON_SY','Carbon Resources Yard',    'Carbon Resources', 'competitor', 'coal'),
    ('DELTA_SY', 'Delta Yard',               'Delta',            'competitor', 'bulk'),
    ('ITL_SY',   'ITL Yard',                 'ITL',              'competitor', 'bulk'),
    ('RCL_SY',   'RCL Yard',                 'RCL',              'competitor', 'bulk'),
    ('STL_SY',   'STL Yard',                 'STL',              'competitor', 'bulk')
ON CONFLICT (yard_code) DO NOTHING;

-- No-Go Zones
INSERT INTO port_map.no_go_zones (zone_name,reason)
VALUES
    ('Defence Area',       'defence_area'),
    ('STP Pond Area',      'restricted'),
    ('Railway R&D Yard',   'railway_crossing'),
    ('Meghadri Gedda',     'water_body'),
    ('Naval Officers Area','defence_area'),
    ('Hindustan Shipyard', 'restricted');
