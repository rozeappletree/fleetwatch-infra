/**
 * VehicleLayer.jsx
 * Renders live truck positions from WebSocket data as Leaflet markers.
 * Each truck gets a rotated SVG arrow icon colored by status.
 * Supports click → popup with telemetry data.
 */
import { useMemo } from 'react'
import { LayerGroup, Marker, Popup, CircleMarker } from 'react-leaflet'
import L from 'leaflet'
import { TRUCK_STATUS_COLORS } from '../layers/layerStyles'

// Build a rotated SVG icon for a truck
function makeTruckIcon(bearing, status) {
  const color = TRUCK_STATUS_COLORS[status] || TRUCK_STATUS_COLORS.ok
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">
    <g transform="rotate(${bearing}, 16, 16)">
      <polygon points="16,2 26,28 16,22 6,28" fill="${color}" stroke="#fff" stroke-width="2"/>
    </g>
  </svg>`
  return L.divIcon({
    html: svg,
    className: '',
    iconSize:   [32, 32],
    iconAnchor: [16, 16],
    popupAnchor:[0, -18],
  })
}

function TruckMarker({ truck, onTruckClick }) {
  const icon = useMemo(
    () => makeTruckIcon(truck.brg || 0, truck._status || 'ok'),
    [truck.brg, truck._status]
  )

  return (
    <>
      {/* Accuracy circle */}
      <CircleMarker
        center={[truck.lat, truck.lng]}
        radius={Math.max(truck.acc || 10, 8)}
        pathOptions={{
          color:       TRUCK_STATUS_COLORS[truck._status] || TRUCK_STATUS_COLORS.ok,
          fillColor:   TRUCK_STATUS_COLORS[truck._status] || TRUCK_STATUS_COLORS.ok,
          fillOpacity: 0.08,
          weight:      1,
          opacity:     0.3,
        }}
      />

      {/* Truck marker */}
      <Marker
        position={[truck.lat, truck.lng]}
        icon={icon}
        eventHandlers={{ click: () => onTruckClick?.(truck) }}
        zIndexOffset={1000}
      >
        <Popup maxWidth={220}>
          <div style={{ fontFamily: 'var(--font)', minWidth: 160 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
              <strong style={{ color: '#22d3ee', fontFamily: 'var(--mono)', fontSize: '0.85rem' }}>
                {truck.vid}
              </strong>
              <span style={{
                padding: '2px 7px',
                borderRadius: 4,
                fontSize: '0.7rem',
                fontWeight: 600,
                background: TRUCK_STATUS_COLORS[truck._status] + '22',
                color:      TRUCK_STATUS_COLORS[truck._status],
                border:     `1px solid ${TRUCK_STATUS_COLORS[truck._status]}44`,
              }}>
                {truck._status || 'ok'}
              </span>
            </div>
            <div style={{ fontSize: '0.72rem', color: '#94a3b8', lineHeight: 1.7 }}>
              <div>Speed: <span style={{ color: '#e2e8f0' }}>{truck.spd?.toFixed(1) ?? '—'} km/h</span></div>
              <div>Bearing: <span style={{ color: '#e2e8f0' }}>{(truck.brg || 0).toFixed(0)}°</span></div>
              <div>Accuracy: <span style={{ color: '#e2e8f0' }}>±{truck.acc?.toFixed(0) ?? '?'}m</span></div>
              {truck.bat != null && <div>Battery: <span style={{ color: '#e2e8f0' }}>{truck.bat}%</span></div>}
              {truck.did && <div>Driver: <span style={{ color: '#e2e8f0', fontFamily: 'var(--mono)' }}>{truck.did}</span></div>}
            </div>
          </div>
        </Popup>
      </Marker>
    </>
  )
}

export default function VehicleLayer({ trucks, onTruckClick }) {
  const truckList = useMemo(() => Array.from(trucks.values()), [trucks])

  if (truckList.length === 0) return null

  return (
    <LayerGroup>
      {truckList.map(truck => (
        <TruckMarker
          key={truck.vid}
          truck={truck}
          onTruckClick={onTruckClick}
        />
      ))}
    </LayerGroup>
  )
}
