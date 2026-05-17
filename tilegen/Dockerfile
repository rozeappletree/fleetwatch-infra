FROM osgeo/gdal:ubuntu-small-3.6.3

# The base image already has ogr2ogr/GDAL. Add PostgreSQL client + build tools.
# osgeo/gdal:ubuntu-small-3.6.3 is based on Ubuntu 22.04 (Jammy) — apt repos are active.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        postgresql-client \
        wget \
        unzip \
        cron \
        git \
        g++ \
        make \
        libsqlite3-dev \
        zlib1g-dev && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Build Tippecanoe 1.36.0 from original mapbox source (no pmtiles, compiles cleanly)
ARG TIPPECANOE_RELEASE="1.36.0"

RUN cd /root \
    && git clone --depth=1 --branch ${TIPPECANOE_RELEASE} https://github.com/mapbox/tippecanoe.git tippecanoe \
    && cd tippecanoe \
    && make -j$(nproc) \
    && make install \
    && cd /root \
    && rm -rf /root/tippecanoe

COPY ./tippecanoe /

RUN crontab crontab.txt

RUN chmod 755 /tilegen.sh /get_static_data.sh /docker-entrypoint.sh /generate_port.sh

CMD './docker-entrypoint.sh'