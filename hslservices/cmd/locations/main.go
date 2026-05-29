package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"

	fleet "github.com/dmw2151/fleetbridge"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/mmcloughlin/geohash"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/semaphore"
)

var (
	ctx = context.Background()

	// NOTE: Replace this with a Real Number of N Max Connections (currently static @ 100)
	apiHandler = LocationsAPIHandler{
		client:      fleet.InitRedisClient(ctx),
		conns:       make([]*upgradedLocationListener, 100),
		sem:         semaphore.NewWeighted(100),
		mu:          sync.Mutex{},
		openIdx:     0,
		unregisterC: make(chan int),
	}

	// Upgrader for WS connections...
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

// LocationsAPIHandler - Responsible for Responding Web <-> LocationsAPI
// <-> Redis requests made to the Locations API endpoints
type LocationsAPIHandler struct {
	client      *redis.Client
	conns       []*upgradedLocationListener
	sem         *semaphore.Weighted
	mu          sync.Mutex
	openIdx     int
	unregisterC chan int
}

// upgradedLocationListener to avoid any blocking on message fanout to client
type upgradedLocationListener struct {
	c  *websocket.Conn
	cH chan []byte
}

// Healthcheck - Nothing More...
func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Write(
		[]byte("Fleet Tracker India — OK"),
	)
}

// httpConnectionUpgrade - To Initialize a Websocket Connection need an upgrade
// function to hijack the original HTTP call, in this case, it's just adding the
// connection to LocationsAPIHandler list of registered connections
func (lh *LocationsAPIHandler) httpConnectionUpgrade(conn *websocket.Conn) {

	// Acquire lock so we can handle for the case where many clients attempt
	// at once, access the lock, pretty sure there's a
	// smarter way to do this connection pooling....

	if lh.sem.TryAcquire(1) {
		lh.mu.Lock()

		ull := &upgradedLocationListener{
			c:  conn,
			cH: make(chan []byte, 10), // Spare them a tiny buffer for each connection to handle for bursts
		}

		lh.conns[lh.openIdx] = ull

		lh.mu.Unlock()

		if err := ull.recv(lh.openIdx, lh.unregisterC); err != nil {
			lh.sem.Release(1)
			conn.Close()
			ull = nil
		}
	} else {
		// NOTE: Write some Connection Overload Error
		conn.WriteMessage(1, []byte("We're At Capacity"))
	}
}

// recv - receive messages to each client forever...
func (ull *upgradedLocationListener) recv(idx int, callbackCh chan int) error {
	for msg := range ull.cH {

		if err := ull.c.WriteMessage(1, msg); err != nil {

			if errors.Is(err, syscall.EPIPE) {
				// If the connection drops; remove the conn by sending the index of the channel
				// to the remove connection handler...
				log.Infof("Sending Unregister %d", idx)
				callbackCh <- idx
				return err
			}
		}
	}

	// Should never happen...Make up an error for here...
	return nil
}

// subscriptionFanout - subscribe to a topic (Redis PUB/SUB) channel and receive
// messages for perpetuity.
//
// For each connection registered on the LocationsAPIHandler, push the message
// along to that connection as well
func (lh *LocationsAPIHandler) subscriptionFanout() {

	// go-redis v9 uses context.Background() — client.Context() was removed in v9
	sub := lh.client.Subscribe(
		ctx, "currentLocationsPS",
	)

	defer func() {
		log.Info("Exit from PUB/SUB Channel")
		sub.Unsubscribe(ctx, "currentLocationsPS")
	}()

	// Open Redis PUB/SUB Channel...
	channel := sub.Channel()
	log.Info("Reading from PUB/SUB Channel")

	for msg := range channel {
		// cast msg -> msgB and then send to all listening connections...
		msgB := []byte(msg.Payload)

		for i, sub := range lh.conns {
			if sub != nil {
				// Never Block!!
				select {
				case sub.cH <- msgB:
				default:
				}
			} else if i < lh.openIdx {
				lh.openIdx = i
			}
		}
	}
	log.Info("Exit from PUB/SUB Channel")
}

func (lh *LocationsAPIHandler) unregisterConnections() {

	for i := range lh.unregisterC {
		log.Infof("Got Unregister Command: %d", i)
		if i > lh.openIdx {
			lh.openIdx = i
		}
	}
}

// livelocationsHandler -
func (lh *LocationsAPIHandler) livelocationsHandler(w http.ResponseWriter, r *http.Request) {

	upgrader.CheckOrigin = func(r *http.Request) bool {
		return true
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Connection", "keep-alive")

	// upgrade this connection to a WebSocket connection
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(err)
	}

	// Note - Check how this behaves on failure...
	lh.httpConnectionUpgrade(ws)

}

func (lh *LocationsAPIHandler) historicallocationsHandler(w http.ResponseWriter, r *http.Request) {

	// Decode trip query from body: expects {"vid":"...", "tid":"..."}
	var req struct {
		VehicleID string `json:"vid"`
		TripID    string `json:"tid"`
	}
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Reconstruct the tripKey the same way main.go does
	tmpEvent := fleet.TruckEvent{VehicleID: req.VehicleID, TripID: req.TripID}
	tripKey := tmpEvent.GetEventHash()

	// TS.MRANGE — pull all aggregated series for this trip
	result, err := lh.client.Do(
		ctx, "TS.MRANGE", "-", "+", "FILTER", fmt.Sprintf("trip=%s", tripKey),
	).Result()

	if err != nil {
		log.Error(err)
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Connection", "keep-alive")

	var (
		respArr = make([]fleet.TruckEvent, 240)
		realLen int
	)

	for _, body := range (result).([]interface{}) {
		keys := (body).([]interface{})
		series, positions := keys[0].(string), keys[2]

		positionsArr := positions.([]interface{})
		realLen = len(positionsArr)

		if strings.HasSuffix(series, "gh:agg") {
			for i, tup := range positionsArr {
				ts, gh := tup.([]interface{})[0].(int64), tup.([]interface{})[1].(string)
				ghI, err := strconv.ParseFloat(gh, 64)
				if err != nil {
					log.Error(err)
				}
				lat, lng := geohash.DecodeIntWithPrecision(uint64(ghI), 64)
				respArr[i] = fleet.TruckEvent{
					Lat:       lat,
					Lng:       lng,
					Timestamp: ts,
					VehicleID: req.VehicleID,
					TripID:    req.TripID,
				}
			}
		}

		if strings.HasSuffix(series, "speed:agg") {
			for i, tup := range positionsArr {
				spd := tup.([]interface{})[1].(string)
				spdf, err := strconv.ParseFloat(spd, 32)
				if err != nil {
					log.Error(err)
				}
				respArr[i].Speed = float32(spdf)
			}
		}
	}

	b, _ := json.Marshal(respArr[:realLen])
	w.Write(b)
}

func init() {

	// Wire os.Getenv into the trips.go helper so it can read PG_DSN.
	osGetenv = os.Getenv

	// Set Logging Config
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)

	log.SetFormatter(&log.TextFormatter{
		DisableColors:   true,
		TimestampFormat: "2006-01-02 15:04:05.0000",
	})

	// Initialise the Port Map PostGIS connection.
	initPortDB()

	// Initialize the LocationsAPI Handler & Have It Subscribe
	// to Target Topics

	go apiHandler.subscriptionFanout()
	go apiHandler.unregisterConnections()
}

func main() {

	router := mux.NewRouter().StrictSlash(true)

	// Healthcheck the API...
	router.HandleFunc("/health/", healthCheck)

	// Live Locations Endpoint...
	router.HandleFunc("/locations/", apiHandler.livelocationsHandler)

	// Historical Locations Endpoint...
	router.HandleFunc("/histlocations/", apiHandler.historicallocationsHandler)

	// GEO Endpoints — live fleet spatial queries
	router.HandleFunc("/fleet/current/", apiHandler.currentFleetHandler)
	router.HandleFunc("/fleet/nearby/", apiHandler.nearbyFleetHandler)

	// Port Map Endpoints (berths, yards, corridors, gates, trip assignment)
	RegisterPortMapRoutes(router)

	log.Fatal(
		http.ListenAndServe(":2152", router),
	)
}
