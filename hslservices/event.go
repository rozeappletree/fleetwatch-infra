package fleetbridge

import (
	"crypto/md5"
	"fmt"
	"io"

	log "github.com/sirupsen/logrus"
)

// TruckEvent represents a single GPS telemetry payload published by a driver's
// Flutter app. Field names use compact JSON keys to minimize bytes over 2G/3G
// networks (e.g. "vid" instead of "vehicle_id").
//
// Payload example:
//
//	{
//	  "vid": "AP 30 Y 1828",   // vehicle registration
//	  "did": "9505683966",      // driver phone / ID
//	  "tid": "uuid-trip-id",   // trip UUID (new per trip)
//	  "lat": 17.7137197,
//	  "lng": 83.1691558,
//	  "spd": 0.30,              // km/h (rounded to 2dp)
//	  "brg": 0.0,               // bearing degrees from north
//	  "acc": 5.10,              // GPS accuracy radius in metres
//	  "ts":  1778132507,        // Unix epoch seconds (UTC)
//	  "bat": 82,                // battery %
//	  "src": "mobile"           // source tag
//	}
type TruckEvent struct {
	VehicleID string  `json:"vid"` // Registration plate, e.g. "AP 30 Y 1828"
	DriverID  string  `json:"did"` // Driver phone / employee ID
	TripID    string  `json:"tid"` // UUID for this trip session
	Lat       float64 `json:"lat"` // WGS84 latitude
	Lng       float64 `json:"lng"` // WGS84 longitude
	Speed     float32 `json:"spd"` // Speed in km/h (rounded to 2dp)
	Bearing   float32 `json:"brg"` // Heading degrees clockwise from north
	Accuracy  float32 `json:"acc"` // GPS fix accuracy in metres
	Timestamp int64   `json:"ts"`  // Unix epoch seconds (UTC)
	Battery   int     `json:"bat"` // Device battery level 0–100
	Source    string  `json:"src"` // "mobile" | "obd" | "sim"
}

// GetEventHash returns an MD5 hex string that uniquely identifies a trip
// session, derived from VehicleID + TripID. Used as the Redis key prefix for
// time-series data so each trip gets its own series.
func (e *TruckEvent) GetEventHash() string {
	h := md5.New()
	io.WriteString(h, fmt.Sprintf("%s:%s", e.VehicleID, e.TripID))
	tripKey := fmt.Sprintf("%x", h.Sum(nil))

	log.Debugf("trip_key vehicle=%s trip=%s hash=%s", e.VehicleID, e.TripID, tripKey)
	return tripKey
}

// EventHolder is the top-level wrapper that the Go worker unmarshals each
// raw MQTT payload into. The Flutter app publishes TruckEvent JSON directly
// (no wrapper envelope), so the holder simply embeds TruckEvent.
type EventHolder struct {
	VP TruckEvent `json:"VP"`
}
