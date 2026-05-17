#!/bin/sh
# get_static_data.sh
# ─────────────────────────────────────────────────────────────────────────────
# Seed the PostGIS database with India geofence boundaries.
#
# For Indian fleet tracking we use OpenStreetMap / GADM boundaries
# instead of the old Helsinki GTFS / postcode shapefiles.
#
# Data sources (free, no registration required):
#   GADM India states:
#     https://geodata.ucdavis.edu/gadm/gadm4.1/shp/gadm41_IND_1.zip
#   GADM India districts:
#     https://geodata.ucdavis.edu/gadm/gadm4.1/shp/gadm41_IND_2.zip
# ─────────────────────────────────────────────────────────────────────────────
set -e

mkdir -p /sources/static/areas/states /sources/static/areas/districts

echo ">>> Downloading India state boundaries (GADM 4.1)..."
wget -q "https://geodata.ucdavis.edu/gadm/gadm4.1/shp/gadm41_IND_1.zip" -O /tmp/states.zip
unzip -q /tmp/states.zip -d /sources/static/areas/states

echo ">>> Downloading India district boundaries (GADM 4.1)..."
wget -q "https://geodata.ucdavis.edu/gadm/gadm4.1/shp/gadm41_IND_2.zip" -O /tmp/districts.zip
unzip -q /tmp/districts.zip -d /sources/static/areas/districts

echo ">>> Loading state boundaries into PostGIS..."
ogr2ogr -a_srs "EPSG:4326" \
    -f "PostgreSQL" \
    -t_srs "EPSG:4326" \
    -overwrite \
    PG:"host=${POSTGRES_HOST} user=${POSTGRES_USER} dbname=${POSTGRES_DB}" \
    /sources/static/areas/states/ \
    -nlt PROMOTE_TO_MULTI \
    -nln india_states

echo ">>> Loading district boundaries into PostGIS..."
ogr2ogr -a_srs "EPSG:4326" \
    -f "PostgreSQL" \
    -t_srs "EPSG:4326" \
    -overwrite \
    PG:"host=${POSTGRES_HOST} user=${POSTGRES_USER} dbname=${POSTGRES_DB}" \
    /sources/static/areas/districts/ \
    -nlt PROMOTE_TO_MULTI \
    -nln india_districts

echo ">>> Done seeding India boundary data."
