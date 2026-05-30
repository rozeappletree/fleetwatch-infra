/**
 * FleetMap.jsx
 * React Leaflet map component. Manages:
 *  - Google Satellite + Labels tile layers (always-on base)
 *  - GeoJSON overlays: railway lines/areas, highway lines
 *  - Live vehicle markers from WebSocket (VehicleLayer)
 *  - Historical trip trail (TrailLayer)
 *  - Layer control for toggling overlays
 *  - flyTo / showTrail exposed via ref for Sidebar interactions
 */
import { useRef, useCallback, forwardRef, useImperativeHandle } from 'react'
import {
  MapContainer,
  TileLayer,
  LayersControl,
  useMap,
  Pane,
} from 'react-leaflet'
import 'leaflet/dist/leaflet.css'

import RailwayLayer  from './components/RailwayLayer'
import HighwayLayer  from './components/HighwayLayer'
import VehicleLayer  from './components/VehicleLayer'
import TrailLayer, { useTrail } from './components/TrailLayer'

const { BaseLayer, Overlay } = LayersControl

// ── Vizag Port default view ──────────────────────────────────────────────────
const DEFAULT_CENTER = [17.7041, 83.2905]
const DEFAULT_ZOOM   = 14

// ── Tile URLs ────────────────────────────────────────────────────────────────
const TILE_SATELLITE = 'https://mt1.google.com/vt/lyrs=s&x={x}&y={y}&z={z}'
const TILE_LABELS    = 'https://mt1.google.com/vt/lyrs=h&x={x}&y={y}&z={z}'
const TILE_OSM       = 'https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png'

// ── FlyTo controller (inner component to access map instance) ─────────────────
function MapController({ controllerRef }) {
  const map = useMap()

  useImperativeHandle(controllerRef, () => ({
    flyTo(lng, lat, zoom = 16) {
      map.flyTo([lat, lng], zoom, { duration: 0.9 })
    },
  }), [map])

  return null
}

// ── Main FleetMap component ───────────────────────────────────────────────────
const FleetMap = forwardRef(function FleetMap({ trucks, onTruckClick }, ref) {
  const controllerRef = useRef(null)
  const { trail, fetchTrail } = useTrail()

  // Expose flyTo + showTrail to parent (App.jsx)
  useImperativeHandle(ref, () => ({
    flyTo(lng, lat, zoom = 16) {
      controllerRef.current?.flyTo(lng, lat, zoom)
    },
    async showTrail(vid, tid) {
      await fetchTrail(vid, tid)
    },
  }), [fetchTrail])

  const handleTruckClick = useCallback((truck) => {
    onTruckClick?.(truck)
    controllerRef.current?.flyTo(truck.lng, truck.lat, 16)
  }, [onTruckClick])

  return (
    <MapContainer
      center={DEFAULT_CENTER}
      zoom={DEFAULT_ZOOM}
      style={{ width: '100%', height: '100%' }}
      zoomControl={true}
      attributionControl={true}
    >
      <MapController controllerRef={controllerRef} />

      {/* ── Base Layers ── */}
      <LayersControl position="topright">
        <BaseLayer checked name="🛰 Satellite + Labels">
          <>
            <TileLayer
              url={TILE_SATELLITE}
              attribution='© Google'
              maxZoom={21}
              tileSize={256}
            />
            {/* Custom pane for labels so they render on top of roads (z-index 450 is above overlayPane) */}
            <Pane name="labelsPane" style={{ zIndex: 450, pointerEvents: 'none' }}>
              <TileLayer
                url={TILE_LABELS}
                attribution=''
                maxZoom={21}
                tileSize={256}
                opacity={0.9}
              />
            </Pane>
          </>
        </BaseLayer>

        <BaseLayer name="🗺 OpenStreetMap">
          <TileLayer
            url={TILE_OSM}
            attribution='© <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
            maxZoom={19}
          />
        </BaseLayer>

        {/* ── GeoJSON Overlays ── */}
        <Overlay checked name="🚂 Railways">
          <RailwayLayer />
        </Overlay>

        <Overlay checked name="🛣 Highways">
          <HighwayLayer />
        </Overlay>

        {/* ── Live Data ── */}
        <Overlay checked name="🚛 Vehicles">
          <VehicleLayer trucks={trucks} onTruckClick={handleTruckClick} />
        </Overlay>
      </LayersControl>

      {/* Trail always rendered outside LayersControl so it persists */}
      <TrailLayer trail={trail} />
    </MapContainer>
  )
})

export default FleetMap
