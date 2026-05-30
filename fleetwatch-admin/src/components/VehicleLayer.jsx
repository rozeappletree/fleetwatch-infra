/**
 * VehicleLayer.jsx
 * Renders live truck positions as MapLibre Markers with rotated SVG arrow icons.
 * Status colors: ok=green, deviation=red, stopped=amber.
 * Click → popup with telemetry data + triggers parent onTruckClick.
 */
import { useMemo, useState } from 'react'
import { Marker, Popup } from 'react-map-gl/maplibre'

const STATUS_COLORS = {
  ok:        '#1D9E75',
  deviation: '#E24B4A',
  stopped:   '#EF9F27',
}

function TruckMarker({ truck, onTruckClick }) {
  const [showPopup, setShowPopup] = useState(false)
  const color = STATUS_COLORS[truck._status] || STATUS_COLORS.ok

  const svg = useMemo(() => (
    `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">
      <g transform="rotate(${truck.brg || 0}, 16, 16)">
        <polygon points="16,2 26,28 16,22 6,28" fill="${color}" stroke="#fff" stroke-width="2"/>
      </g>
    </svg>`
  ), [truck.brg, color])

  return (
    <>
      <Marker
        longitude={truck.lng}
        latitude={truck.lat}
        onClick={() => { onTruckClick?.(truck); setShowPopup(true) }}
        style={{ cursor: 'pointer' }}
      >
        <div dangerouslySetInnerHTML={{ __html: svg }} style={{ width: 32, height: 32 }} />
      </Marker>

      {showPopup && (
        <Popup
          longitude={truck.lng}
          latitude={truck.lat}
          anchor="bottom"
          onClose={() => setShowPopup(false)}
          closeButton={true}
          maxWidth="220px"
        >
          <div style={{ fontFamily: 'monospace', minWidth: 160 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
              <strong style={{ color: '#22d3ee', fontSize: '0.85rem' }}>{truck.vid}</strong>
              <span style={{
                padding: '2px 7px', borderRadius: 4, fontSize: '0.7rem', fontWeight: 600,
                background: color + '22', color, border: `1px solid ${color}44`,
              }}>
                {truck._status || 'ok'}
              </span>
            </div>
            <div style={{ fontSize: '0.72rem', color: '#94a3b8', lineHeight: 1.7 }}>
              <div>Speed:    <span style={{ color: '#e2e8f0' }}>{truck.spd?.toFixed(1) ?? '—'} km/h</span></div>
              <div>Bearing:  <span style={{ color: '#e2e8f0' }}>{(truck.brg || 0).toFixed(0)}°</span></div>
              <div>Accuracy: <span style={{ color: '#e2e8f0' }}>±{truck.acc?.toFixed(0) ?? '?'}m</span></div>
              {truck.bat != null && <div>Battery: <span style={{ color: '#e2e8f0' }}>{truck.bat}%</span></div>}
              {truck.did && <div>Driver: <span style={{ color: '#e2e8f0', fontFamily: 'monospace' }}>{truck.did}</span></div>}
            </div>
          </div>
        </Popup>
      )}
    </>
  )
}

export default function VehicleLayer({ trucks, onTruckClick }) {
  const truckList = useMemo(() => Array.from(trucks.values()), [trucks])
  if (truckList.length === 0) return null
  return truckList.map(truck => (
    <TruckMarker key={truck.vid} truck={truck} onTruckClick={onTruckClick} />
  ))
}
