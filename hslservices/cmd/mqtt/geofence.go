package main

// =============================================================================
// geofence.go
// In-memory geofence engine for the Vizag Port fleet worker.
//
// Loads all polygon layers from PostGIS at startup, refreshes every 5 minutes.
// All point-in-polygon checks are pure in-memory — zero DB hits at runtime.
//
// Four alert checks per incoming GPS event (in priority order):
//   1. Competitor yard entry   → PUBLISH alerts:competitor_yard
//   2. No-go zone entry        → PUBLISH alerts:no_go_zone
//   3. Route deviation (3+ consecutive outside corridor) → PUBLISH alerts:deviation
//   4. Extended stop (45 s, ignoring gates/weighbridges) → PUBLISH alerts:stop
//
// Library: github.com/paulmach/orb v0.11.0
// =============================================================================

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/planar"
	redis "github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

// =============================================================================
// Data types
// =============================================================================

// namedPolygon is a polygon with human-readable metadata loaded from PostGIS.
type namedPolygon struct {
	ID    int
	Name  string
	Extra string // owner (competitor yards) or reason (no-go zones)
	Ring  orb.Ring
}

// namedCorridor is a route corridor polygon with route metadata.
type namedCorridor struct {
	ID         int
	Name       string
	SourceName string
	DestName   string
	Ring       orb.Ring
}

// namedPoint is a gate or weighbridge used for stop-suppression proximity checks.
type namedPoint struct {
	ID   int
	Name string
	Pt   orb.Point
}

// vehicleState holds per-vehicle mutable state for deviation and stop detection.
type vehicleState struct {
	// Route deviation
	consecutiveViolations int

	// Stop detection
	stopStarted *time.Time
	stopAlerted bool
}

// =============================================================================
// GeofenceEngine
// =============================================================================

// GeofenceEngine holds all polygon/point sets and per-vehicle state.
// All public methods are goroutine-safe.
type GeofenceEngine struct {
	// Polygon datasets — swapped atomically on refresh
	mu              sync.RWMutex
	competitorYards []namedPolygon
	noGoZones       []namedPolygon
	corridors       []namedCorridor
	knownPoints     []namedPoint // gates + weighbridges

	// Per-vehicle state
	stateMu sync.Mutex
	state   map[string]*vehicleState

	db    *sql.DB
	rdb   *redis.Client
	ctx   context.Context
}

// NewGeofenceEngine creates the engine, loads from PostGIS, and starts the
// background refresh goroutine. Returns an error if the initial load fails.
func NewGeofenceEngine(ctx context.Context, db *sql.DB, rdb *redis.Client) (*GeofenceEngine, error) {
	e := &GeofenceEngine{
		state: make(map[string]*vehicleState),
		db:    db,
		rdb:   rdb,
		ctx:   ctx,
	}

	if err := e.reload(); err != nil {
		return nil, fmt.Errorf("geofence initial load failed: %w", err)
	}

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := e.reload(); err != nil {
					log.Warnf("geofence: refresh failed: %v", err)
				}
			}
		}
	}()

	return e, nil
}

// reload fetches all polygon layers from PostGIS and atomically replaces the
// in-memory datasets. Safe to call concurrently — uses a separate RW lock swap.
func (e *GeofenceEngine) reload() error {
	log.Info("geofence: reloading polygons from PostGIS")

	compYards, err := polygonQuery(e.db, "competitor_yards", "name", "owner")
	if err != nil {
		return fmt.Errorf("competitor_yards: %w", err)
	}

	noGoZones, err := polygonQuery(e.db, "no_go_zones", "name", "reason")
	if err != nil {
		return fmt.Errorf("no_go_zones: %w", err)
	}

	corridors, err := loadCorridors(e.db)
	if err != nil {
		return fmt.Errorf("route_corridors: %w", err)
	}

	knownPts, err := loadKnownPoints(e.db)
	if err != nil {
		return fmt.Errorf("gates/weighbridges: %w", err)
	}

	e.mu.Lock()
	e.competitorYards = compYards
	e.noGoZones = noGoZones
	e.corridors = corridors
	e.knownPoints = knownPts
	e.mu.Unlock()

	log.WithFields(log.Fields{
		"competitor_yards": len(compYards),
		"no_go_zones":      len(noGoZones),
		"corridors":        len(corridors),
		"known_points":     len(knownPts),
	}).Info("geofence: reload complete")

	return nil
}

// =============================================================================
// PostGIS loaders
// =============================================================================

// polygonQuery loads all rows from `table` as namedPolygon using ST_AsGeoJSON.
// nameCol and extraCol name the text columns to read.
func polygonQuery(db *sql.DB, table, nameCol, extraCol string) ([]namedPolygon, error) {
	q := fmt.Sprintf(
		`SELECT id,
		        COALESCE(%s, '') AS name,
		        COALESCE(%s, '') AS extra,
		        ST_AsGeoJSON(
		            ST_ForcePolygonCW(
		                ST_Transform(wkb_geometry, 4326)
		            )
		        ) AS geojson
		 FROM %s`,
		nameCol, extraCol, table,
	)

	rows, err := db.Query(q)
	if err != nil {
		// Table may not exist yet (pre-QGIS); return empty, not error
		log.Warnf("geofence: query %s: %v — returning empty", table, err)
		return nil, nil
	}
	defer rows.Close()

	var out []namedPolygon
	for rows.Next() {
		var p namedPolygon
		var gjson string
		if err := rows.Scan(&p.ID, &p.Name, &p.Extra, &gjson); err != nil {
			return nil, err
		}
		ring, err := geojsonToRing(gjson)
		if err != nil {
			log.Warnf("geofence: %s id=%d bad geometry: %v — skip", table, p.ID, err)
			continue
		}
		p.Ring = ring
		out = append(out, p)
	}
	return out, rows.Err()
}

// loadCorridors loads active route corridors from PostGIS.
func loadCorridors(db *sql.DB) ([]namedCorridor, error) {
	q := `SELECT id,
	             COALESCE(name,'')        AS name,
	             COALESCE(source_name,'') AS source_name,
	             COALESCE(dest_name,'')   AS dest_name,
	             ST_AsGeoJSON(
	                 ST_ForcePolygonCW(corridor)
	             ) AS geojson
	      FROM route_corridors
	      WHERE active = true`

	rows, err := db.Query(q)
	if err != nil {
		log.Warnf("geofence: query route_corridors: %v — returning empty", err)
		return nil, nil
	}
	defer rows.Close()

	var out []namedCorridor
	for rows.Next() {
		var c namedCorridor
		var gjson string
		if err := rows.Scan(&c.ID, &c.Name, &c.SourceName, &c.DestName, &gjson); err != nil {
			return nil, err
		}
		ring, err := geojsonToRing(gjson)
		if err != nil {
			log.Warnf("geofence: corridor id=%d bad geometry: %v — skip", c.ID, err)
			continue
		}
		c.Ring = ring
		out = append(out, c)
	}
	return out, rows.Err()
}

// loadKnownPoints loads active gates and weighbridges for stop-suppression.
func loadKnownPoints(db *sql.DB) ([]namedPoint, error) {
	q := `SELECT id,
	             COALESCE(name,'') AS name,
	             ST_X(wkb_geometry) AS lng,
	             ST_Y(wkb_geometry) AS lat
	      FROM (
	          SELECT id, name, wkb_geometry FROM gates       WHERE active = true
	          UNION ALL
	          SELECT id, name, wkb_geometry FROM weighbridges WHERE active = true
	      ) pts`

	rows, err := db.Query(q)
	if err != nil {
		log.Warnf("geofence: query gates/weighbridges: %v — returning empty", err)
		return nil, nil
	}
	defer rows.Close()

	var out []namedPoint
	for rows.Next() {
		var p namedPoint
		var lng, lat float64
		if err := rows.Scan(&p.ID, &p.Name, &lng, &lat); err != nil {
			return nil, err
		}
		p.Pt = orb.Point{lng, lat}
		out = append(out, p)
	}
	return out, rows.Err()
}

// geojsonToRing parses a GeoJSON Polygon string and returns the exterior ring
// as an orb.Ring. Coordinates are in [lng, lat] order (GeoJSON standard).
func geojsonToRing(gjson string) (orb.Ring, error) {
	var geom struct {
		Type        string        `json:"type"`
		Coordinates [][][]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal([]byte(gjson), &geom); err != nil {
		return nil, err
	}
	if geom.Type != "Polygon" || len(geom.Coordinates) == 0 {
		return nil, fmt.Errorf("expected Polygon, got %q", geom.Type)
	}
	exterior := geom.Coordinates[0]
	ring := make(orb.Ring, len(exterior))
	for i, c := range exterior {
		if len(c) < 2 {
			return nil, fmt.Errorf("short coordinate at index %d", i)
		}
		ring[i] = orb.Point{c[0], c[1]} // [lng, lat]
	}
	return ring, nil
}

// =============================================================================
// CheckEvent — main entry point called per GPS event from writeRedis()
// =============================================================================

// CheckEvent runs all four geofence checks for one GPS position.
// Safe to call from multiple goroutines simultaneously.
func (e *GeofenceEngine) CheckEvent(vid string, lat, lng float64, speed float32, ts int64) {
	// ── Coordinate bounds sanity check (Vizag port area) ────────────────────
	if lng < 83.270 || lng > 83.320 || lat < 17.690 || lat > 17.730 {
		log.Warnf("geofence: vid=%s pos=[%f,%f] outside Vizag bounds — skip", vid, lng, lat)
		return
	}

	pt := orb.Point{lng, lat} // [lng, lat] — GeoJSON / orb convention

	// Snapshot current datasets under read lock
	e.mu.RLock()
	compYards := e.competitorYards
	noGoZones := e.noGoZones
	corridors := e.corridors
	knownPts := e.knownPoints
	e.mu.RUnlock()

	// ── 1. Competitor yard (highest priority) ─────────────────────────────────
	for _, yard := range compYards {
		if planar.RingContains(yard.Ring, pt) {
			e.publish("alerts:competitor_yard", map[string]interface{}{
				"vid":       vid,
				"lat":       lat,
				"lng":       lng,
				"ts":        ts,
				"yard_name": yard.Name,
				"owner":     yard.Extra,
			})
			log.WithField("vid", vid).Warnf("🚨 competitor yard entry: %s (%s)", yard.Name, yard.Extra)
			break
		}
	}

	// ── 2. No-go zone ─────────────────────────────────────────────────────────
	for _, zone := range noGoZones {
		if planar.RingContains(zone.Ring, pt) {
			e.publish("alerts:no_go_zone", map[string]interface{}{
				"vid":       vid,
				"lat":       lat,
				"lng":       lng,
				"ts":        ts,
				"zone_name": zone.Name,
				"reason":    zone.Extra,
			})
			log.WithField("vid", vid).Warnf("⛔ no-go zone entry: %s (%s)", zone.Name, zone.Extra)
			break
		}
	}

	// ── 3. Route corridor deviation ───────────────────────────────────────────
	e.checkDeviation(vid, pt, lat, lng, ts, corridors)

	// ── 4. Stop detection ─────────────────────────────────────────────────────
	e.checkStop(vid, lat, lng, ts, speed, knownPts)
}

// =============================================================================
// Check implementations
// =============================================================================

// checkDeviation fires alerts:deviation after 3 consecutive out-of-corridor points.
func (e *GeofenceEngine) checkDeviation(
	vid string, pt orb.Point, lat, lng float64, ts int64,
	corridors []namedCorridor,
) {
	if len(corridors) == 0 {
		return
	}

	insideAny := false
	for _, c := range corridors {
		if planar.RingContains(c.Ring, pt) {
			insideAny = true
			break
		}
	}

	e.stateMu.Lock()
	st := e.vehicleState(vid)
	if insideAny {
		st.consecutiveViolations = 0
		e.stateMu.Unlock()
		return
	}
	st.consecutiveViolations++
	count := st.consecutiveViolations
	e.stateMu.Unlock()

	if count >= 3 {
		// Use the first corridor name as context
		routeName := corridors[0].Name
		e.publish("alerts:deviation", map[string]interface{}{
			"vid":             vid,
			"lat":             lat,
			"lng":             lng,
			"ts":              ts,
			"route_name":      routeName,
			"violation_count": count,
		})
		log.WithField("vid", vid).Warnf("📍 route deviation: %d consecutive violations", count)
	}
}

// checkStop fires alerts:stop after the vehicle is stationary >45 s,
// unless it is within 30 m of a known gate or weighbridge.
func (e *GeofenceEngine) checkStop(
	vid string, lat, lng float64, ts int64, speed float32,
	knownPts []namedPoint,
) {
	const (
		stopThresholdKPH = 2.0  // km/h — below this = "stopped"
		alertAfterSec    = 45
		suppressRadiusM  = 30.0 // metres — suppress near gate/weighbridge
	)

	e.stateMu.Lock()
	st := e.vehicleState(vid)

	if float64(speed) >= stopThresholdKPH {
		st.stopStarted = nil
		st.stopAlerted = false
		e.stateMu.Unlock()
		return
	}

	now := time.Now()
	if st.stopStarted == nil {
		st.stopStarted = &now
		st.stopAlerted = false
		e.stateMu.Unlock()
		return
	}

	elapsed := now.Sub(*st.stopStarted)
	alreadyAlerted := st.stopAlerted
	e.stateMu.Unlock()

	if alreadyAlerted || elapsed < time.Duration(alertAfterSec)*time.Second {
		return
	}

	// Suppress if near a known point
	for _, kp := range knownPts {
		if haversineM(lat, lng, kp.Pt[1], kp.Pt[0]) <= suppressRadiusM {
			log.WithField("vid", vid).Debugf("stop suppressed: near %s", kp.Name)
			return
		}
	}

	e.stateMu.Lock()
	st.stopAlerted = true
	e.stateMu.Unlock()

	e.publish("alerts:stop", map[string]interface{}{
		"vid":              vid,
		"lat":              lat,
		"lng":              lng,
		"ts":               ts,
		"duration_seconds": int(elapsed.Seconds()),
	})
	log.WithField("vid", vid).Warnf("⏸ vehicle stopped %.0f s", elapsed.Seconds())
}

// =============================================================================
// Helpers
// =============================================================================

// vehicleState returns (or creates) the mutable state for vid.
// Caller MUST hold stateMu.
func (e *GeofenceEngine) vehicleState(vid string) *vehicleState {
	if st, ok := e.state[vid]; ok {
		return st
	}
	st := &vehicleState{}
	e.state[vid] = st
	return st
}

// publish encodes payload as JSON and publishes to a Redis Pub/Sub channel.
func (e *GeofenceEngine) publish(channel string, payload map[string]interface{}) {
	b, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("geofence: marshal %s: %v", channel, err)
		return
	}
	if err := e.rdb.Publish(e.ctx, channel, string(b)).Err(); err != nil {
		log.Errorf("geofence: publish %s: %v", channel, err)
	}
}

// haversineM returns the great-circle distance in metres between two WGS-84
// coordinates. Used for stop-suppression proximity checks.
func haversineM(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6_371_000.0
	φ1 := lat1 * math.Pi / 180
	φ2 := lat2 * math.Pi / 180
	Δφ := (lat2 - lat1) * math.Pi / 180
	Δλ := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
