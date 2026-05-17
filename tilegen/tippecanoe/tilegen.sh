#!/bin/bash
# =============================================================================
# generate_tiles.sh  (updated — adds Vizag port layers)
# Runs on cron every 5 minutes (see tilegen/tippecanoe/crontab.txt).
#
# What this script does:
#   1. Refreshes event_log materialized view (existing statistics layer)
#   2. Dumps district-level speed stats GeoJSON → tiles/statistics/ (existing)
#   3. Dumps all Vizag port layers → /tmp/
#   4. Re-tiles port layers into /tiles/port.mbtiles via tippecanoe
# =============================================================================
set -euo pipefail

PG_HOST="${POSTGRES_HOST:-postgis}"
PG_DB="${POSTGRES_DB:-fleet}"
PG_USER="${POSTGRES_USER:-postgres}"
PG_PASS="${POSTGRES_PASSWORD:-pass}"

PG="PG:host=${PG_HOST} dbname=${PG_DB} user=${PG_USER} password=${PG_PASS}"
PSQL="psql -h ${PG_HOST} -U ${PG_USER} -d ${PG_DB}"

# ── PART A: existing statistics layer (unchanged) ─────────────────────────────

mkdir -p /sources/agg/

PGPASSWORD="${PG_PASS}" ${PSQL} \
    -c "REFRESH MATERIALIZED VIEW CONCURRENTLY event_log;" 2>/dev/null || true

rm -f /sources/agg/statistics.geojson
ogr2ogr -f GeoJSON /sources/agg/statistics.geojson \
    "PG:host=${PG_HOST} dbname=${PG_DB} user=${PG_USER} password=${PG_PASS}" \
    -nlt PROMOTE_TO_MULTI \
    -geomfield geom \
    -sql "
        SELECT
            d2.name_2       AS district,
            d2.name_1       AS state,
            d2.wkb_geometry AS geom,
            stats.spd
        FROM (
            SELECT ogc_fid, AVG(spd) AS spd
            FROM india_districts d
            LEFT JOIN event_log ON ST_Intersects(d.wkb_geometry, event_log.geom)
            GROUP BY ogc_fid
        ) stats
        LEFT JOIN india_districts d2 USING (ogc_fid);
    " 2>/dev/null || true

if [ -f /sources/agg/statistics.geojson ] && \
   [ "$(stat -c %s /sources/agg/statistics.geojson)" -gt "100" ]; then
    tippecanoe -L statistics:/sources/agg/statistics.geojson \
        --no-tile-compression \
        --force \
        -e /tiles/statistics
fi

# ── PART B: Vizag Port layers → port.mbtiles ─────────────────────────────────

echo "[tilegen] Dumping Vizag Port layers — $(date)"

dump_layer() {
    local table="$1"
    local dest="/tmp/${table}.geojson"

    # Check table exists and has rows before dumping
    COUNT=$(PGPASSWORD="${PG_PASS}" ${PSQL} -tAc \
        "SELECT COUNT(*) FROM information_schema.tables \
         WHERE table_name='${table}' AND table_schema='public';" 2>/dev/null || echo 0)

    if [ "${COUNT}" -eq "0" ]; then
        echo "  [SKIP] ${table} table not found — using empty placeholder"
        echo '{"type":"FeatureCollection","features":[]}' > "${dest}"
        return
    fi

    ROWS=$(PGPASSWORD="${PG_PASS}" ${PSQL} -tAc \
        "SELECT COUNT(*) FROM ${table};" 2>/dev/null || echo 0)

    if [ "${ROWS}" -eq "0" ]; then
        echo "  [SKIP] ${table} is empty — using empty placeholder"
        echo '{"type":"FeatureCollection","features":[]}' > "${dest}"
        return
    fi

    echo "  [DUMP] ${table} (${ROWS} rows)"
    ogr2ogr -f GeoJSON "${dest}" "${PG}" "${table}"
}

# Standard port layers
dump_layer "port_boundary"
dump_layer "berths"
dump_layer "client_yards"
dump_layer "competitor_yards"
dump_layer "truck_roads"
dump_layer "gates"
dump_layer "weighbridges"
dump_layer "no_go_zones"

# Route corridors — only active ones, with selected properties
CORRIDORS_COUNT=$(PGPASSWORD="${PG_PASS}" ${PSQL} -tAc \
    "SELECT COUNT(*) FROM route_corridors WHERE active = true;" 2>/dev/null || echo 0)

if [ "${CORRIDORS_COUNT}" -gt "0" ]; then
    echo "  [DUMP] route_corridors (${CORRIDORS_COUNT} active)"
    ogr2ogr -f GeoJSON /tmp/corridors.geojson "${PG}" \
        -sql "SELECT id, name, source_name, dest_name, corridor AS geom
              FROM route_corridors WHERE active = true"
else
    echo '{"type":"FeatureCollection","features":[]}' > /tmp/corridors.geojson
fi

# ── Generate port.mbtiles ─────────────────────────────────────────────────────
echo "[tilegen] Running tippecanoe → /tiles/port.mbtiles"

tippecanoe \
    --output=/tiles/port.mbtiles \
    --layer=port_boundary    /tmp/port_boundary.geojson \
    --layer=berths           /tmp/berths.geojson \
    --layer=client_yards     /tmp/client_yards.geojson \
    --layer=competitor_yards /tmp/competitor_yards.geojson \
    --layer=truck_roads      /tmp/truck_roads.geojson \
    --layer=gates            /tmp/gates.geojson \
    --layer=weighbridges     /tmp/weighbridges.geojson \
    --layer=no_go_zones      /tmp/no_go_zones.geojson \
    --layer=corridors        /tmp/corridors.geojson \
    --minimum-zoom=12 \
    --maximum-zoom=19 \
    --simplification=2 \
    --force

echo "[tilegen] Port tiles regenerated: $(date)"

# ── PART C: VPA Port Map tiles (generate_port.sh) ──────────────────────────
# generate_port.sh re-exports port_map.* layers and runs tippecanoe
# to produce /tiles/port.mbtiles.
if [ -f /generate_port.sh ]; then
    echo "[tilegen] Running generate_port.sh — $(date)"
    /generate_port.sh
else
    echo "[tilegen] WARN: /generate_port.sh not found — skipping port tiles"
fi