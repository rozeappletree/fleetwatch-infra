package main

// =============================================================================
// main.go  (updated — adds geofence engine startup + CheckEvent per event)
// =============================================================================

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	fleet "github.com/dmw2151/fleetbridge"
	_ "github.com/lib/pq"
	"github.com/mmcloughlin/geohash"
	redis "github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

var (
	msgBroker   = fleet.NewMsgBroker(1024)
	ctx, cancel = context.WithCancel(context.Background())
	_           = fleet.InitMQTTClient(msgBroker)
	redisClient = fleet.InitRedisClient(ctx)
	nWorkers    = 10

	// geofence is initialised in main() after DB connection is ready.
	// Workers call CheckEvent; nil check guards the pre-init window.
	geofence *GeofenceEngine
)

// openPostGIS opens a connection to the PostGIS database using environment
// variables that match those injected by docker-compose (postgres.env).
func openPostGIS() (*sql.DB, error) {
	host := getenv("POSTGRES_HOST", "postgis")
	port := getenv("POSTGRES_PORT", "5432")
	user := getenv("POSTGRES_USER", "postgres")
	pass := getenv("POSTGRES_PASSWORD", "pass")
	dbname := getenv("POSTGRES_DB", "fleet")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, pass, dbname,
	)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping postgis: %w", err)
	}
	return db, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// statTripID checks if a tripKey already has a time-series in Redis.
// Returns true if it already exists (SADD returned 0 = element was present).
func statTripID(client *redis.Client, key string, tripKey string) bool {
	resp, err := client.Do(ctx, "SADD", key, tripKey).Result()
	if err != nil {
		return false
	}
	return resp.(int64) == 0
}

// createTimeSeriesPair creates a raw + aggregated time-series pair for one
// telemetry metric (e.g. "speed" or "gh") keyed on tripKey.
func createTimeSeriesPair(client *redis.Client, tripKey string, label string) {
	pipe := client.TxPipeline()

	pipe.Do(ctx, "TS.CREATE", fmt.Sprintf("positions:%s:%s", tripKey, label))
	pipe.Do(
		ctx, "TS.CREATE",
		fmt.Sprintf("positions:%s:%s:agg", tripKey, label),
		"RETENTION", 120*60*1000,
		"LABELS", label, 1, "trip", tripKey,
	)

	if _, err := pipe.Exec(ctx); err != nil {
		log.WithFields(log.Fields{
			"TripKey": tripKey,
			"Series":  fmt.Sprintf("positions:%s:%s", tripKey, label),
		}).Warn("TS.CREATE failed (may already exist): ", err)
	}

	pipe.Do(
		ctx, "TS.CREATERULE",
		fmt.Sprintf("positions:%s:%s", tripKey, label),
		fmt.Sprintf("positions:%s:%s:agg", tripKey, label),
		"AGGREGATION", "LAST", 15000,
	)

	if _, err := pipe.Exec(ctx); err != nil {
		log.WithFields(log.Fields{
			"TripKey": tripKey,
		}).Warn("TS.CREATERULE failed: ", err)
	}
}

// writeRedis drains the staging channel and writes each TruckEvent to Redis:
//
//  1. PUBLISH to "currentLocationsPS" (WebSocket dashboard fan-out)
//  2. XADD to "events" stream (write-behind to PostGIS via RedisGears)
//  3. TS.ADD speed and geohash time-series (last 60 s for history API)
//  4. GeofenceEngine.CheckEvent — four in-memory geofence checks
func writeRedis(ctx context.Context, C <-chan []byte, client *redis.Client) {
	for msg := range C {

		e := &fleet.EventHolder{}
		if err := fleet.DeserializeMQTTBody(msg, e); err != nil {
			switch err := err.(type) {
			case *fleet.MQTTValidationError:
				log.WithField("body", string(msg)).Debug("Validation: ", err)
			default:
				log.WithField("body", string(msg)).Debug("Deserialize: ", err)
			}
			continue
		}

		v := e.VP // shorthand for the TruckEvent
		tripKey := v.GetEventHash()

		// Ensure time-series exist for this trip (idempotent after first call)
		if !statTripID(client, "tripKeys", tripKey) {
			log.WithFields(log.Fields{
				"Vehicle": v.VehicleID,
				"Trip":    v.TripID,
				"Key":     tripKey,
			}).Info("New trip registered — creating time-series")

			createTimeSeriesPair(client, tripKey, "speed")
			createTimeSeriesPair(client, tripKey, "gh")
		}

		pipe := client.TxPipeline()

		// 1. Pub/Sub fan-out → WebSocket dashboard
		pipe.Publish(ctx, "currentLocationsPS", msg)

		// 2. Stream → PostGIS write-behind (RedisGears)
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: "events",
			Values: []interface{}{
				"vid", v.VehicleID,
				"did", v.DriverID,
				"tid", v.TripID,
				"key", tripKey,
				"lat", v.Lat,
				"lng", v.Lng,
				"spd", v.Speed,
				"brg", v.Bearing,
				"acc", v.Accuracy,
				"ts", v.Timestamp,
				"bat", v.Battery,
				"src", v.Source,
			},
		})

		// 3. Time-series: speed (km/h) — 60 s rolling
		pipe.Do(ctx,
			"TS.ADD", fmt.Sprintf("positions:%s:speed", tripKey),
			"*", v.Speed,
			"RETENTION", 60*1000,
			"CHUNK_SIZE", 16,
			"ON_DUPLICATE", "LAST",
		)

		// 3b. Time-series: geohash (int64) — 60 s rolling
		pipe.Do(ctx,
			"TS.ADD", fmt.Sprintf("positions:%s:gh", tripKey),
			"*", geohash.EncodeIntWithPrecision(v.Lat, v.Lng, 64),
			"RETENTION", 60*1000,
			"ON_DUPLICATE", "LAST",
		)

		if _, err := pipe.Exec(ctx); err != nil {
			if netErr, ok := err.(net.Error); ok {
				log.Errorf("Redis network error: %+v", netErr)
			}
			log.WithField("TripKey", tripKey).Errorf("Redis pipeline failed: %+v", err)
		} else {
			log.WithFields(log.Fields{
				"Vehicle": v.VehicleID,
				"Trip":    tripKey,
				"Speed":   v.Speed,
			}).Debug("Event written")
		}

		// 4. Geofence checks (in-memory, zero DB hits)
		if geofence != nil {
			geofence.CheckEvent(v.VehicleID, v.Lat, v.Lng, v.Speed, v.Timestamp)
		}
	}
}

func init() {
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.WarnLevel) // Change to InfoLevel for verbose output
}

func main() {
	defer cancel()

	// ── Connect to PostGIS and start geofence engine ──────────────────────────
	db, err := openPostGIS()
	if err != nil {
		log.Warnf("geofence: PostGIS unavailable (%v) — geofence disabled", err)
	} else {
		gf, err := NewGeofenceEngine(ctx, db, redisClient)
		if err != nil {
			log.Warnf("geofence: engine init failed (%v) — geofence disabled", err)
		} else {
			geofence = gf
			log.Info("geofence: engine started")
		}
	}

	// ── Drain staging channel with N concurrent Redis writers ─────────────────
	for i := 0; i < nWorkers; i++ {
		go writeRedis(ctx, msgBroker.StagingC, redisClient)
	}

	quitChannel := make(chan os.Signal, 1)
	signal.Notify(quitChannel, syscall.SIGINT, syscall.SIGTERM)
	<-quitChannel
	log.Info("Fleet Go Worker — shutting down")
}
