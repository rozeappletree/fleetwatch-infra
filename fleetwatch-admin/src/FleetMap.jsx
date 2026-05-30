/**
 * FleetMap.jsx
 * MapLibre GL map via react-map-gl. Manages:
 *  - OSM raster base (desaturated) + 5 Martin vector tile sources (mapStyle.json)
 *  - Live vehicle markers from WebSocket (VehicleLayer)
 *  - Historical trip trail (TrailLayer)
 *  - flyTo / showTrail exposed via ref for App.jsx interactions
 */
import { useRef, useCallback, useState, forwardRef, useImperativeHandle } from 'react'
import { Map } from 'react-map-gl/maplibre'
import 'maplibre-gl/dist/maplibre-gl.css'
import mapStyle from './mapStyle.json'
import VehicleLayer from './components/VehicleLayer'
import TrailLayer, { useTrail } from './components/TrailLayer'

const DEFAULT_VIEW = { longitude: 83.2905, latitude: 17.7041, zoom: 14 }

const TOGGLE_BTN = {
  position: 'absolute', top: 10, left: 10, zIndex: 10,
  display: 'flex', borderRadius: 6, overflow: 'hidden',
  boxShadow: '0 2px 6px rgba(0,0,0,0.5)',
  fontFamily: 'sans-serif', fontSize: 12, fontWeight: 600,
}
const btnStyle = (active) => ({
  padding: '6px 12px', cursor: 'pointer', border: 'none',
  background: active ? '#0C4A9E' : '#1e293b',
  color: active ? '#fff' : '#94a3b8',
  transition: 'background 0.15s',
})

const FleetMap = forwardRef(function FleetMap({ trucks, onTruckClick }, ref) {
  const mapRef = useRef(null)
  const { trail, fetchTrail } = useTrail()
  const [basemap, setBasemap] = useState('osm')

  useImperativeHandle(ref, () => ({
    flyTo(lng, lat, zoom = 16) {
      mapRef.current?.flyTo({ center: [lng, lat], zoom, duration: 900 })
    },
    async showTrail(vid, tid) {
      await fetchTrail(vid, tid)
    },
  }), [fetchTrail])

  const handleTruckClick = useCallback((truck) => {
    onTruckClick?.(truck)
    mapRef.current?.flyTo({ center: [truck.lng, truck.lat], zoom: 16, duration: 900 })
  }, [onTruckClick])

  const switchBasemap = useCallback((mode) => {
    const map = mapRef.current?.getMap()
    if (!map) return
    if (mode === 'satellite') {
      map.setLayoutProperty('base-tiles', 'visibility', 'none')
      map.setLayoutProperty('base-google-hybrid', 'visibility', 'visible')
    } else {
      map.setLayoutProperty('base-google-hybrid', 'visibility', 'none')
      map.setLayoutProperty('base-tiles', 'visibility', 'visible')
    }
    setBasemap(mode)
  }, [])

  return (
    <div style={{ position: 'relative', width: '100%', height: '100%' }}>
      <Map
        ref={mapRef}
        initialViewState={DEFAULT_VIEW}
        style={{ width: '100%', height: '100%' }}
        mapStyle={mapStyle}
        attributionControl={true}
      >
        <VehicleLayer trucks={trucks} onTruckClick={handleTruckClick} />
        <TrailLayer trail={trail} />
      </Map>

      <div style={TOGGLE_BTN}>
        <button style={btnStyle(basemap === 'osm')}       onClick={() => switchBasemap('osm')}>🗺 OSM</button>
        <button style={btnStyle(basemap === 'satellite')} onClick={() => switchBasemap('satellite')}>🛰 Satellite</button>
      </div>
    </div>
  )
})

export default FleetMap
