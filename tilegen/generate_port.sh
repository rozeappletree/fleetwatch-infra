#!/bin/bash
# =============================================================================
# generate_port.sh
# FleetTrack India — VPA Port Map tile generation
#
# Reads all port_map.* layers from PostGIS and produces /tiles/port.mbtiles
# via tippecanoe.
#
# Designed to run inside the tilegen container (osgeo/gdal + tippecanoe).
#
# Environment variables (with defaults):
#   POSTGRES_HOST     postgis
#   POSTGRES_DB       fleet
#   POSTGRES_USER     postgres
#   POSTGRES_PASSWORD pass
#
# Usage:
#   docker exec tilegen /generate_port.sh
# Or invoked from tilegen.sh cron job automatically.
# =============================================================================
set -euo pipefail

PG_HOST="${POSTGRES_HOST:-postgis}"
PG_DB="${POSTGRES_DB:-fleet}"
PG_USER="${POSTGRES_USER:-postgres}"
PG_PASS="${POSTGRES_PASSWORD:-pass}"

PG="PG:host=${PG_HOST} dbname=${PG_DB} user=${PG_USER} password=${PG_PASS}"
PSQL="PGPASSWORD=${PG_PASS} psql -h ${PG_HOST} -U ${PG_USER} -d ${PG_DB}"

TILES_DIR="/tiles"
TMP_DIR="/tmp/port_map"
OUTPUT="${TILES_DIR}/port.mbtiles"

mkdir -p "${TMP_DIR}" "${TILES_DIR}"

echo "============================================================"
echo "[generate_port] Starting VPA port map tile generation"
echo "[generate_port] $(date)"
echo "============================================================"

# ----------------------------------------------------------------------------
# Helper: dump a single port_map table to GeoJSON
# Writes an empty FeatureCollection if the table is missing or has no rows.
# ----------------------------------------------------------------------------
dump_layer() {
    local schema_table="$1"    # e.g. port_map.berths
    local layer="${schema_table##*.}"  # strip schema prefix → berths
    local dest="${TMP_DIR}/${layer}.geojson"
    local sql="${2:-}"         # optional SQL override

    # Check table has at least one row with a non-null geometry
    local rows
    rows=$(${PSQL} -tAc \
        "SELECT COUNT(*) FROM ${schema_table} WHERE geom IS NOT NULL;" \
        2>/dev/null || echo 0)
    rows=$(echo "${rows}" | tr -d '[:space:]')

    if [ "${rows}" = "0" ]; then
        echo "  [SKIP] ${schema_table} — no geometries yet (using empty placeholder)"
        echo '{"type":"FeatureCollection","features":[]}' > "${dest}"
        return 0
    fi

    echo "  [DUMP] ${schema_table} (${rows} rows with geom)"

    if [ -n "${sql}" ]; then
        ogr2ogr -f GeoJSON "${dest}" "${PG}" -sql "${sql}" -geomfield geom
    else
        ogr2ogr -f GeoJSON "${dest}" "${PG}" "${schema_table}"
    fi
}

# ----------------------------------------------------------------------------
# Export all port_map layers
# ----------------------------------------------------------------------------

echo ""
echo "--- Exporting layers from port_map schema ---"

dump_layer "port_map.port_boundary"

dump_layer "port_map.vpa_zones"

dump_layer "port_map.berths"

dump_layer "port_map.yards"

dump_layer "port_map.port_roads"

dump_layer "port_map.gates"

dump_layer "port_map.weighbridges"

dump_layer "port_map.no_go_zones"

# Route corridors — export active only, using the corridor polygon as geom
echo "  [DUMP] port_map.route_corridors (active only)"
CORR_ROWS=$(${PSQL} -tAc \
    "SELECT COUNT(*) FROM port_map.route_corridors
     WHERE active = true AND corridor IS NOT NULL;" \
    2>/dev/null || echo 0)
CORR_ROWS=$(echo "${CORR_ROWS}" | tr -d '[:space:]')

if [ "${CORR_ROWS}" = "0" ]; then
    echo "  [SKIP] route_corridors — no active corridors with geometry yet"
    echo '{"type":"FeatureCollection","features":[]}' > "${TMP_DIR}/route_corridors.geojson"
else
    ogr2ogr -f GeoJSON "${TMP_DIR}/route_corridors.geojson" "${PG}" \
        -sql "SELECT
                  id,
                  corridor_code,
                  corridor_name,
                  entry_gate,
                  exit_gate,
                  buffer_meters,
                  road_sequence::TEXT AS road_sequence,
                  corridor AS geom
              FROM port_map.route_corridors
              WHERE active = true AND corridor IS NOT NULL" \
        -geomfield geom
fi

# ----------------------------------------------------------------------------
# Generate port.mbtiles via tippecanoe
#   zoom  12–19: sufficient for port operations (z15 = ~4 m/px)
#   simplification 2: gentle — preserves berth polygon accuracy
# ----------------------------------------------------------------------------

echo ""
echo "--- Generating ${OUTPUT} ---"

tippecanoe \
    --output="${OUTPUT}" \
    --layer=port_boundary   "${TMP_DIR}/port_boundary.geojson" \
    --layer=vpa_zones       "${TMP_DIR}/vpa_zones.geojson" \
    --layer=berths          "${TMP_DIR}/berths.geojson" \
    --layer=yards           "${TMP_DIR}/yards.geojson" \
    --layer=port_roads      "${TMP_DIR}/port_roads.geojson" \
    --layer=gates           "${TMP_DIR}/gates.geojson" \
    --layer=weighbridges    "${TMP_DIR}/weighbridges.geojson" \
    --layer=no_go_zones     "${TMP_DIR}/no_go_zones.geojson" \
    --layer=corridors       "${TMP_DIR}/route_corridors.geojson" \
    --minimum-zoom=12 \
    --maximum-zoom=19 \
    --simplification=2 \
    --force

echo ""
echo "============================================================"
echo "[generate_port] Port tiles regenerated → ${OUTPUT}"
echo "[generate_port] $(date)"
echo "============================================================"
