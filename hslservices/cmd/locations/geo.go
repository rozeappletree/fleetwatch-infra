package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	redis "github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

var validGeoUnits = map[string]bool{"m": true, "km": true, "mi": true, "ft": true}

// currentFleetHandler — GET /fleet/current/
//
// Returns all trucks whose truck:{vid} hash is alive (last ping within 5 min).
// Uses fleet:live GEO key for membership, truck:{vid} hashes for state.
// Trucks whose hash has expired are excluded and lazily removed from fleet:live.
func (lh *LocationsAPIHandler) currentFleetHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	members, err := lh.client.ZRange(ctx, "fleet:live", 0, -1).Result()
	if err != nil {
		log.Errorf("[geo] ZRANGE fleet:live: %v", err)
		http.Error(w, `{"error":"redis error"}`, http.StatusInternalServerError)
		return
	}
	if len(members) == 0 {
		w.Write([]byte("[]"))
		return
	}

	// Pipeline HGETALL for all known members — empty result = truck offline
	pipe := lh.client.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(members))
	for i, vid := range members {
		cmds[i] = pipe.HGetAll(ctx, "truck:"+vid)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		log.Errorf("[geo] pipeline HGETALL: %v", err)
		http.Error(w, `{"error":"redis error"}`, http.StatusInternalServerError)
		return
	}

	type truckState struct {
		VehicleID string  `json:"vid"`
		Lat       float64 `json:"lat"`
		Lng       float64 `json:"lng"`
		Speed     float64 `json:"spd"`
		Bearing   float64 `json:"brg"`
		Timestamp int64   `json:"ts"`
		Battery   int     `json:"bat"`
		TripID    string  `json:"tid"`
		DriverID  string  `json:"did"`
	}

	result := make([]truckState, 0, len(members))
	var stale []interface{}

	for i, vid := range members {
		state, err := cmds[i].Result()
		if err != nil || len(state) == 0 {
			stale = append(stale, vid) // TTL expired — mark for lazy removal
			continue
		}
		result = append(result, truckState{
			VehicleID: vid,
			Lat:       geoParseFloat(state["lat"]),
			Lng:       geoParseFloat(state["lng"]),
			Speed:     geoParseFloat(state["spd"]),
			Bearing:   geoParseFloat(state["brg"]),
			Timestamp: geoParseInt64(state["ts"]),
			Battery:   int(geoParseInt64(state["bat"])),
			TripID:    state["tid"],
			DriverID:  state["did"],
		})
	}

	// Lazy cleanup: remove members whose truck hash has expired from the GEO
	// key so fleet:live doesn't grow unboundedly with historical vehicles.
	// Done in a goroutine so it doesn't block the response.
	if len(stale) > 0 {
		go func() {
			if err := lh.client.ZRem(ctx, "fleet:live", stale...).Err(); err != nil {
				log.Warnf("[geo] ZRem stale members: %v", err)
			}
		}()
	}

	json.NewEncoder(w).Encode(result)
}

// nearbyFleetHandler — GET /fleet/nearby/?lat=17.70&lng=83.29&radius=5000&unit=m
//
// Returns online trucks within radius of a point using Redis GEOSEARCH.
// Query params:
//   lat, lng  — center point (WGS84); lat ∈ [-90,90], lng ∈ [-180,180]
//   radius    — search radius, must be > 0
//   unit      — m | km | mi | ft (default: m)
//
// Dist field in response is in the requested unit.
// Offline trucks (truck:{vid} hash expired) are excluded and lazily cleaned up.
func (lh *LocationsAPIHandler) nearbyFleetHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	q := r.URL.Query()

	lat, err := strconv.ParseFloat(q.Get("lat"), 64)
	if err != nil || lat < -90 || lat > 90 {
		http.Error(w, `{"error":"lat must be a number in [-90, 90]"}`, http.StatusBadRequest)
		return
	}
	lng, err := strconv.ParseFloat(q.Get("lng"), 64)
	if err != nil || lng < -180 || lng > 180 {
		http.Error(w, `{"error":"lng must be a number in [-180, 180]"}`, http.StatusBadRequest)
		return
	}
	radius, err := strconv.ParseFloat(q.Get("radius"), 64)
	if err != nil || radius <= 0 {
		http.Error(w, `{"error":"radius must be a positive number"}`, http.StatusBadRequest)
		return
	}
	unit := q.Get("unit")
	if unit == "" {
		unit = "m"
	}
	if !validGeoUnits[unit] {
		http.Error(w, `{"error":"unit must be one of: m, km, mi, ft"}`, http.StatusBadRequest)
		return
	}

	locations, err := lh.client.GeoSearchLocation(ctx, "fleet:live", &redis.GeoSearchLocationQuery{
		GeoSearchQuery: redis.GeoSearchQuery{
			Longitude:  lng,
			Latitude:   lat,
			Radius:     radius,
			RadiusUnit: unit,
			Sort:       "ASC",
		},
		WithCoord: true,
		WithDist:  true,
	}).Result()
	if err != nil {
		log.Errorf("[geo] GEOSEARCH fleet:live: %v", err)
		http.Error(w, `{"error":"redis error"}`, http.StatusInternalServerError)
		return
	}
	if len(locations) == 0 {
		w.Write([]byte("[]"))
		return
	}

	// Filter offline trucks: pipeline HGETALL, skip those with empty result
	pipe := lh.client.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, len(locations))
	for i, loc := range locations {
		cmds[i] = pipe.HGetAll(ctx, "truck:"+loc.Name)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		log.Errorf("[geo] pipeline HGETALL (nearby): %v", err)
		http.Error(w, `{"error":"redis error"}`, http.StatusInternalServerError)
		return
	}

	type nearbyTruck struct {
		VehicleID string  `json:"vid"`
		Lat       float64 `json:"lat"`
		Lng       float64 `json:"lng"`
		Dist      float64 `json:"dist"`
		DistUnit  string  `json:"dist_unit"`
		Speed     float64 `json:"spd"`
		Bearing   float64 `json:"brg"`
		Timestamp int64   `json:"ts"`
		TripID    string  `json:"tid"`
	}

	result := make([]nearbyTruck, 0, len(locations))
	var stale []interface{}

	for i, loc := range locations {
		state, err := cmds[i].Result()
		if err != nil || len(state) == 0 {
			stale = append(stale, loc.Name)
			continue
		}
		result = append(result, nearbyTruck{
			VehicleID: loc.Name,
			Lat:       loc.Latitude,
			Lng:       loc.Longitude,
			Dist:      loc.Dist,
			DistUnit:  unit,
			Speed:     geoParseFloat(state["spd"]),
			Bearing:   geoParseFloat(state["brg"]),
			Timestamp: geoParseInt64(state["ts"]),
			TripID:    state["tid"],
		})
	}

	if len(stale) > 0 {
		go func() {
			if err := lh.client.ZRem(ctx, "fleet:live", stale...).Err(); err != nil {
				log.Warnf("[geo] ZRem stale members (nearby): %v", err)
			}
		}()
	}

	json.NewEncoder(w).Encode(result)
}

func geoParseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func geoParseInt64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
