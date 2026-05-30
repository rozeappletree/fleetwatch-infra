import { useState, useRef, useEffect, useCallback } from 'react'
import FleetMap from './FleetMap'
import useFleetWS, { checkVizagBounds } from './hooks/useFleetWS'
import './App.css'

function lerp(a, b, t) {
  return a + (b - a) * t
}

function lerpBearing(a, b, t) {
  let diff = ((b - a + 540) % 360) - 180
  return (a + diff * t + 360) % 360
}

export default function App() {
  const [online, setOnline]             = useState(false)
  const [trucks, setTrucks]             = useState(new Map())
  const [events, setEvents]             = useState([])
  const [alerts, setAlerts]             = useState([])
  const [selectedTruck, setSelectedTruck] = useState(null)

  // Ref to the FleetMap imperative API
  const mapRef = useRef(null)

  // Maps to hold raw state for interpolation
  const targetsRef = useRef(new Map()) // vid → target state
  const currentRef = useRef(new Map()) // vid → current interpolated state
  const renderRef  = useRef(null)

  // ── Interpolation Loop ──────────────────────────────────────────────────────
  useEffect(() => {
    let lastTime = performance.now()

    function animate(time) {
      const dt = time - lastTime
      lastTime = time

      let changed = false
      const current  = currentRef.current
      const targets  = targetsRef.current
      const newTrucks = new Map(current)

      for (const [vid, target] of targets.entries()) {
        const curr = current.get(vid)

        if (!curr) {
          newTrucks.set(vid, { ...target })
          current.set(vid, { ...target })
          changed = true
          continue
        }

        const speed = 1.0 / 2000.0 // reach target in 2 s
        const t     = Math.min(dt * speed, 1.0)

        const distSq = (target.lat - curr.lat) ** 2 + (target.lng - curr.lng) ** 2
        if (distSq > 0.000000001 || Math.abs(target.brg - curr.brg) > 1) {
          const updated = {
            ...target,
            lat: lerp(curr.lat, target.lat, t),
            lng: lerp(curr.lng, target.lng, t),
            brg: lerpBearing(curr.brg, target.brg, t),
          }
          newTrucks.set(vid, updated)
          current.set(vid, updated)
          changed = true
        } else if (curr._status !== target._status) {
          const updated = { ...target, lat: curr.lat, lng: curr.lng, brg: curr.brg }
          newTrucks.set(vid, updated)
          current.set(vid, updated)
          changed = true
        }
      }

      if (changed) setTrucks(newTrucks)
      renderRef.current = requestAnimationFrame(animate)
    }

    renderRef.current = requestAnimationFrame(animate)
    return () => cancelAnimationFrame(renderRef.current)
  }, [])

  // ── WebSocket Handlers ──────────────────────────────────────────────────────
  const handleEvent = useCallback((ev) => {
    checkVizagBounds(ev.lng, ev.lat)

    const targets = targetsRef.current
    const prev    = targets.get(ev.vid) || {}
    targets.set(ev.vid, {
      ...prev,
      ...ev,
      _status: prev._status || 'ok',
    })

    setEvents(prevEvts => [ev, ...prevEvts].slice(0, 20))
  }, [])

  useFleetWS({
    onEvent:        handleEvent,
    onStatusChange: setOnline,
    onRoute:        () => {},
  })

  const activeAlertsCount = Array.from(targetsRef.current.values()).filter(t => t._status === 'deviation').length
  const stoppedCount      = Array.from(targetsRef.current.values()).filter(t => t._status === 'stopped').length

  // ── Auto-dismiss alerts ─────────────────────────────────────────────────────
  useEffect(() => {
    const interval = setInterval(() => {
      setAlerts(prev => prev.filter(a => Date.now() - a.timestamp < 30000))
    }, 1000)
    return () => clearInterval(interval)
  }, [])

  const handleTruckClick = useCallback((truck) => {
    setSelectedTruck(truck)
    // mapRef flyTo is called inside FleetMap's handleTruckClick already
  }, [])

  const handleEventClick = useCallback((ev) => {
    mapRef.current?.flyTo(ev.lng, ev.lat, 16)
    setSelectedTruck(ev)
  }, [])

  const handleAlertClick = useCallback((alert) => {
    mapRef.current?.flyTo(alert.lng, alert.lat, 16)
  }, [])

  return (
    <div className="app-layout">
      {/* ── ALERTS BANNER ── */}
      {alerts.length > 0 && (
        <div className="alert-banner-container">
          {alerts.map(a => (
            <div
              key={a.id}
              className={`alert-banner ${a.type}`}
              onClick={() => handleAlertClick(a)}
            >
              {a.type === 'deviation' && `⚠ ROUTE DEVIATION — Truck ${a.vid} — ${new Date(a.timestamp).toLocaleTimeString()}`}
              {a.type === 'stopped'   && `⏸ STOPPED — Truck ${a.vid} — ${a.duration} seconds`}
            </div>
          ))}
        </div>
      )}

      {/* ── MAP ── */}
      <div className="map-container">
        <FleetMap
          ref={mapRef}
          trucks={trucks}
          onTruckClick={handleTruckClick}
        />
      </div>

      {/* ── SIDEBAR ── */}
      <div className="sidebar">
        <div className="sidebar-header">
          <div className="brand">
            <span className="logo-icon">🚛</span>
            <div>
              <h1>FleetTrack India</h1>
              <div className="subtitle">Vizag Port Pipeline</div>
            </div>
          </div>
          <div className={`ws-badge ${online ? 'online' : 'offline'}`}>
            <span className="dot"></span>
            {online ? 'Live' : 'Reconnecting…'}
          </div>
        </div>

        <div className="stats-grid">
          <div className="stat-card">
            <div className="stat-label">Total Trucks</div>
            <div className="stat-value">{trucks.size}</div>
          </div>
          <div className="stat-card alert">
            <div className="stat-label">Active Alerts</div>
            <div className="stat-value text-red">{activeAlertsCount}</div>
          </div>
          <div className="stat-card warning">
            <div className="stat-label">Stopped</div>
            <div className="stat-value text-amber">{stoppedCount}</div>
          </div>
        </div>

        {selectedTruck && (
          <div className="selected-truck-card">
            <div className="selected-label">Selected</div>
            <div className="selected-vid">{selectedTruck.vid}</div>
            <button
              className="trail-btn"
              onClick={() => mapRef.current?.showTrail(selectedTruck.vid, selectedTruck.tid)}
            >
              Show Trip Trail
            </button>
          </div>
        )}

        <div className="feed-header">
          <h3>Live Feed</h3>
        </div>

        <div className="feed-list">
          {events.length === 0 ? (
            <div className="empty-feed">Waiting for telemetry...</div>
          ) : (
            events.map((ev, i) => (
              <div
                key={ev.vid + i}
                className="feed-item"
                onClick={() => handleEventClick(ev)}
              >
                <div className="feed-top">
                  <span className="vid">{ev.vid}</span>
                  <span className="time">{new Date(ev.ts * 1000).toLocaleTimeString()}</span>
                </div>
                <div className="feed-chips">
                  <span className="chip">spd: {ev.spd?.toFixed(1)}</span>
                  <span className="chip">acc: ±{ev.acc?.toFixed(0)}m</span>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
