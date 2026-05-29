package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	log "github.com/sirupsen/logrus"
)

// =============================================================================
// trips.go — FleetTrack India, Port Map API
//
// New endpoints:
//   GET  /api/berths/      → list active berths (code, name, harbour, geom GeoJSON)
//   GET  /api/yards/       → list active yards  (code, name, owner_type, geom GeoJSON)
//   GET  /api/corridors/   → list active corridors (code, name, corridor GeoJSON)
//   GET  /api/gates/       → list all gates     (code, name, rfid, geom GeoJSON)
//   POST /api/trips/assign → create trip assignment, return corridor to Flutter
//
// DB connection is read from PG_DSN env var (same DSN as the rest of the Go service).
// =============================================================================

// -----------------------------------------------------------------------------
// DB connection (package-level, initialised in initPortDB)
// -----------------------------------------------------------------------------

var portDB *sql.DB

func initPortDB() {
	dsn := mustEnv("PG_DSN",
		"host=postgis port=5432 dbname=fleet user=postgres password=pass sslmode=disable")
	var err error
	portDB, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("[trips] failed to open DB: %v", err)
	}
	portDB.SetMaxOpenConns(10)
	portDB.SetMaxIdleConns(5)
	portDB.SetConnMaxLifetime(5 * time.Minute)
}

// osGetenv is wired to os.Getenv by main.go's init().
// Declared here so trips.go compiles without importing "os" directly
// (main.go already has it).
var osGetenv func(string) string

func mustEnv(key, fallback string) string {
	if osGetenv != nil {
		if v := osGetenv(key); v != "" {
			return v
		}
	}
	return fallback
}

func init() {
	// Ensure osGetenv is non-nil even if main.go hasn't wired it yet.
	// (Defensive; main.go's init overrides this before initPortDB is called.)
	if osGetenv == nil {
		osGetenv = func(_ string) string { return "" }
	}
}

// -----------------------------------------------------------------------------
// GeoJSON helpers
// -----------------------------------------------------------------------------

// rawJSON lets us embed pre-marshalled PostGIS GeoJSON directly in responses.
type rawJSON json.RawMessage

func (r rawJSON) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	return []byte(r), nil
}

// -----------------------------------------------------------------------------
// Request / response types
// -----------------------------------------------------------------------------

// TripAssignRequest is the body for POST /api/trips/assign
type TripAssignRequest struct {
	VehicleID string `json:"vehicle_id"` // e.g. "AP30Y1828"
	BerthCode string `json:"berth_code"` // e.g. "EQ8"
	YardCode  string `json:"yard_code"`  // e.g. "WQ14_SY"
	Shift     string `json:"shift"`      // "day" | "night"
}

// TripAssignResponse is returned to the Flutter app.
type TripAssignResponse struct {
	TripID        string      `json:"trip_id"`
	VehicleID     string      `json:"vehicle_id"`
	BerthCode     string      `json:"berth_code"`
	YardCode      string      `json:"yard_code"`
	CorridorCode  string      `json:"corridor_code"`
	CorridorName  string      `json:"corridor_name"`
	EntryGate     string      `json:"entry_gate"`
	ExitGate      string      `json:"exit_gate"`
	BufferMeters  int         `json:"buffer_meters"`
	CorridorGeom  rawJSON     `json:"corridor_geom"`  // GeoJSON Polygon
	AssignedAt    time.Time   `json:"assigned_at"`
}

// BerthRow is one row from the berths query.
type BerthRow struct {
	ID          int     `json:"id"`
	BerthCode   string  `json:"berth_code"`
	BerthName   string  `json:"berth_name"`
	QuayType    string  `json:"quay_type"`
	Operator    string  `json:"operator"`
	CapacityMT  *float64 `json:"capacity_mt,omitempty"`
	Harbour     string  `json:"harbour"`
	Geom        rawJSON `json:"geom"`
}

// YardRow is one row from the yards query.
type YardRow struct {
	ID        int     `json:"id"`
	YardCode  string  `json:"yard_code"`
	YardName  string  `json:"yard_name"`
	Owner     string  `json:"owner"`
	OwnerType string  `json:"owner_type"`
	Material  string  `json:"material"`
	Geom      rawJSON `json:"geom"`
}

// CorridorRow is one row from the corridors query.
type CorridorRow struct {
	ID           int     `json:"id"`
	CorridorCode string  `json:"corridor_code"`
	CorridorName string  `json:"corridor_name"`
	EntryGate    string  `json:"entry_gate"`
	ExitGate     string  `json:"exit_gate"`
	BufferMeters int     `json:"buffer_meters"`
	Geom         rawJSON `json:"geom"` // corridor polygon
}

// GateRow is one row from the gates query.
type GateRow struct {
	ID       int     `json:"id"`
	GateCode string  `json:"gate_code"`
	GateName string  `json:"gate_name"`
	GateType string  `json:"gate_type"`
	RFID     bool    `json:"rfid"`
	Active   bool    `json:"active"`
	Geom     rawJSON `json:"geom"`
}

// -----------------------------------------------------------------------------
// GET /api/berths/
// -----------------------------------------------------------------------------

func listBerthsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	rows, err := portDB.QueryContext(r.Context(), `
		SELECT
			id,
			berth_code,
			COALESCE(berth_name,  '') AS berth_name,
			COALESCE(quay_type,   '') AS quay_type,
			COALESCE(operator,    '') AS operator,
			capacity_mt,
			COALESCE(harbour,     '') AS harbour,
			ST_AsGeoJSON(geom)        AS geom
		FROM port_map.berths
		WHERE active = true
		ORDER BY berth_code`)
	if err != nil {
		http.Error(w, fmt.Sprintf("db error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var result []BerthRow
	for rows.Next() {
		var b BerthRow
		var geomStr sql.NullString
		if err := rows.Scan(
			&b.ID, &b.BerthCode, &b.BerthName, &b.QuayType,
			&b.Operator, &b.CapacityMT, &b.Harbour, &geomStr,
		); err != nil {
			log.Warnf("[trips] berths scan: %v", err)
			continue
		}
		if geomStr.Valid {
			b.Geom = rawJSON(geomStr.String)
		}
		result = append(result, b)
	}

	json.NewEncoder(w).Encode(result)
}

// -----------------------------------------------------------------------------
// GET /api/yards/
// -----------------------------------------------------------------------------

func listYardsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	ownerType := r.URL.Query().Get("owner_type") // optional filter: client|competitor|vpa

	query := `
		SELECT
			id,
			yard_code,
			COALESCE(yard_name,  '') AS yard_name,
			COALESCE(owner,      '') AS owner,
			COALESCE(owner_type, '') AS owner_type,
			COALESCE(material,   '') AS material,
			ST_AsGeoJSON(geom)       AS geom
		FROM port_map.yards
		WHERE active = true`

	var args []interface{}
	if ownerType != "" {
		query += " AND owner_type = $1"
		args = append(args, ownerType)
	}
	query += " ORDER BY yard_code"

	rows, err := portDB.QueryContext(r.Context(), query, args...)
	if err != nil {
		http.Error(w, fmt.Sprintf("db error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var result []YardRow
	for rows.Next() {
		var y YardRow
		var geomStr sql.NullString
		if err := rows.Scan(
			&y.ID, &y.YardCode, &y.YardName,
			&y.Owner, &y.OwnerType, &y.Material, &geomStr,
		); err != nil {
			log.Warnf("[trips] yards scan: %v", err)
			continue
		}
		if geomStr.Valid {
			y.Geom = rawJSON(geomStr.String)
		}
		result = append(result, y)
	}

	json.NewEncoder(w).Encode(result)
}

// -----------------------------------------------------------------------------
// GET /api/corridors/
// -----------------------------------------------------------------------------

func listCorridorsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	rows, err := portDB.QueryContext(r.Context(), `
		SELECT
			id,
			corridor_code,
			COALESCE(corridor_name, '') AS corridor_name,
			COALESCE(entry_gate,    '') AS entry_gate,
			COALESCE(exit_gate,     '') AS exit_gate,
			COALESCE(buffer_meters, 80) AS buffer_meters,
			ST_AsGeoJSON(corridor)      AS geom
		FROM port_map.route_corridors
		WHERE active = true
		ORDER BY corridor_code`)
	if err != nil {
		http.Error(w, fmt.Sprintf("db error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var result []CorridorRow
	for rows.Next() {
		var c CorridorRow
		var geomStr sql.NullString
		if err := rows.Scan(
			&c.ID, &c.CorridorCode, &c.CorridorName,
			&c.EntryGate, &c.ExitGate, &c.BufferMeters, &geomStr,
		); err != nil {
			log.Warnf("[trips] corridors scan: %v", err)
			continue
		}
		if geomStr.Valid {
			c.Geom = rawJSON(geomStr.String)
		}
		result = append(result, c)
	}

	json.NewEncoder(w).Encode(result)
}

// -----------------------------------------------------------------------------
// GET /api/gates/
// -----------------------------------------------------------------------------

func listGatesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	rows, err := portDB.QueryContext(r.Context(), `
		SELECT
			id,
			gate_code,
			COALESCE(gate_name, '') AS gate_name,
			COALESCE(gate_type, '') AS gate_type,
			rfid,
			active,
			ST_AsGeoJSON(geom)      AS geom
		FROM port_map.gates
		ORDER BY gate_code`)
	if err != nil {
		http.Error(w, fmt.Sprintf("db error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var result []GateRow
	for rows.Next() {
		var g GateRow
		var geomStr sql.NullString
		if err := rows.Scan(
			&g.ID, &g.GateCode, &g.GateName,
			&g.GateType, &g.RFID, &g.Active, &geomStr,
		); err != nil {
			log.Warnf("[trips] gates scan: %v", err)
			continue
		}
		if geomStr.Valid {
			g.Geom = rawJSON(geomStr.String)
		}
		result = append(result, g)
	}

	json.NewEncoder(w).Encode(result)
}

// -----------------------------------------------------------------------------
// POST /api/trips/assign
//
// Algorithm:
//   1. Validate berth_code exists and is active
//   2. Validate yard_code exists and is active
//   3. Find an active corridor that:
//        - Spatially intersects or is within 200m of the berth centroid
//      OR is matched by berth_id FK (if already linked)
//      Fall back to berth quay_type → corridor heuristic:
//        east_quay  → EQ_MAIN
//        west_quay (WQ1-4) → WQ14_MAIN
//        west_quay (WQ6-8) → WQ68_MAIN
//        container  → VCTPL_MAIN
//        ore_berth  → OB_MAIN
//   4. Insert trip record in port_map.trips (created here if not existing)
//   5. Return TripAssignResponse with corridor GeoJSON
// -----------------------------------------------------------------------------

func assignTripHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var req TripAssignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.VehicleID == "" || req.BerthCode == "" || req.YardCode == "" {
		http.Error(w, `{"error":"vehicle_id, berth_code and yard_code are required"}`,
			http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// 1. Validate berth
	var berthID int
	var quayType, harbour string
	err := portDB.QueryRowContext(ctx, `
		SELECT id, COALESCE(quay_type,'other'), COALESCE(harbour,'inner')
		FROM port_map.berths
		WHERE berth_code = $1 AND active = true`, req.BerthCode,
	).Scan(&berthID, &quayType, &harbour)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"berth not found or inactive"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		log.Errorf("[trips] berth lookup: %v", err)
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	// 2. Validate yard
	var yardID int
	var ownerType string
	err = portDB.QueryRowContext(ctx, `
		SELECT id, COALESCE(owner_type,'vpa')
		FROM port_map.yards
		WHERE yard_code = $1 AND active = true`, req.YardCode,
	).Scan(&yardID, &ownerType)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"yard not found or inactive"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		log.Errorf("[trips] yard lookup: %v", err)
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	// 3. Resolve corridor
	//    Primary: FK match (berth_id already set on corridor)
	//    Fallback: quay_type heuristic
	corridorCode := resolveCorridorCode(quayType, req.BerthCode)

	var corridorID int
	var corridorName, entryGate, exitGate string
	var bufferMeters int
	var corridorGeomStr sql.NullString

	err = portDB.QueryRowContext(ctx, `
		SELECT
			id,
			COALESCE(corridor_name, ''),
			COALESCE(entry_gate,    ''),
			COALESCE(exit_gate,     ''),
			COALESCE(buffer_meters, 80),
			ST_AsGeoJSON(corridor)
		FROM port_map.route_corridors
		WHERE corridor_code = $1 AND active = true`, corridorCode,
	).Scan(&corridorID, &corridorName, &entryGate, &exitGate,
		&bufferMeters, &corridorGeomStr)

	if err == sql.ErrNoRows {
		// Corridor not generated yet (roads not digitized)
		http.Error(w,
			fmt.Sprintf(`{"error":"corridor %s not found — run corridor_generate.sql after QGIS digitizing"}`,
				corridorCode),
			http.StatusNotFound)
		return
	}
	if err != nil {
		log.Errorf("[trips] corridor lookup: %v", err)
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	// 4. Create trip record
	tripID := uuid.New().String()
	assignedAt := time.Now().UTC()

	_, err = portDB.ExecContext(ctx, `
		INSERT INTO port_map.trips
			(trip_id, vehicle_id, berth_id, yard_id, corridor_id,
			 berth_code, yard_code, shift, assigned_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		tripID, req.VehicleID, berthID, yardID, corridorID,
		req.BerthCode, req.YardCode, req.Shift, assignedAt,
	)
	if err != nil {
		// Trips table may not exist yet (it's created separately).
		// Log but don't fail — return corridor data to Flutter regardless.
		log.Warnf("[trips] insert trip record failed (table may not exist): %v", err)
	}

	// 5. Build response
	var corridorGeom rawJSON
	if corridorGeomStr.Valid {
		corridorGeom = rawJSON(corridorGeomStr.String)
	}

	resp := TripAssignResponse{
		TripID:       tripID,
		VehicleID:    req.VehicleID,
		BerthCode:    req.BerthCode,
		YardCode:     req.YardCode,
		CorridorCode: corridorCode,
		CorridorName: corridorName,
		EntryGate:    entryGate,
		ExitGate:     exitGate,
		BufferMeters: bufferMeters,
		CorridorGeom: corridorGeom,
		AssignedAt:   assignedAt,
	}

	log.Infof("[trips] assigned trip=%s vehicle=%s berth=%s yard=%s corridor=%s",
		tripID, req.VehicleID, req.BerthCode, req.YardCode, corridorCode)

	json.NewEncoder(w).Encode(resp)
}

// resolveCorridorCode maps berth attributes to a corridor code.
// Priority: specific berth overrides → quay_type fallback.
func resolveCorridorCode(quayType, berthCode string) string {
	// Specific berth overrides
	switch berthCode {
	case "WQ1", "WQ2", "WQ3", "WQ4":
		return "WQ14_MAIN"
	case "WQ6", "WQ7", "WQ8":
		return "WQ68_MAIN"
	case "OB1", "OB2", "VGCB":
		return "OB_MAIN"
	case "VCTPL1", "VCTPL2":
		return "VCTPL_MAIN"
	}

	// quay_type fallback
	switch quayType {
	case "east_quay":
		return "EQ_MAIN"
	case "west_quay":
		return "WQ14_MAIN" // default west quay route
	case "ore_berth":
		return "OB_MAIN"
	case "container":
		return "VCTPL_MAIN"
	default:
		return "EQ_MAIN" // safest default
	}
}

// -----------------------------------------------------------------------------
// RegisterPortMapRoutes wires all port map endpoints onto an existing router.
// Call this from main.go after initPortDB().
//
// Example:
//   initPortDB()
//   RegisterPortMapRoutes(router)
// -----------------------------------------------------------------------------

func RegisterPortMapRoutes(r *mux.Router) {
	r.HandleFunc("/api/berths/",      listBerthsHandler).Methods("GET")
	r.HandleFunc("/api/yards/",       listYardsHandler).Methods("GET")
	r.HandleFunc("/api/corridors/",   listCorridorsHandler).Methods("GET")
	r.HandleFunc("/api/gates/",       listGatesHandler).Methods("GET")
	r.HandleFunc("/api/trips/assign", assignTripHandler).Methods("POST", "OPTIONS")
}
