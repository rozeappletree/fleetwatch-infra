# Redis Stack — includes RedisTimeSeries, RedisJSON, RediSearch, RedisBloom
# Official image from Redis Ltd (replaces the archived redislabs/redismod)
# https://hub.docker.com/r/redis/redis-stack-server
FROM redis/redis-stack-server:7.4.0-v3

# Switch to root briefly to install Python 3.12 + pip for RedisGears scripts
USER root

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        python3 \
        python3-pip \
        python3-setuptools \
        libpq-dev \
        curl && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Upgrade pip to a version that supports --break-system-packages
RUN python3 -m pip install --upgrade pip --break-system-packages || \
    python3 -m pip install --upgrade pip

# Copy config and gears scripts
COPY ./ /redis/

# Install Python deps for write-behind gears
RUN python3 -m pip install --no-cache-dir --break-system-packages \
        psycopg2-binary==2.9.10 \
        redis==5.2.1 \
        numpy==2.2.2 \
        click==8.1.8

