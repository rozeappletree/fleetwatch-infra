/**
 * TrailLayer.jsx
 * Renders the historical trip trail for a selected truck as a MapLibre line layer.
 * Data fetched from /histlocations/ POST endpoint.
 *
 * useTrail() hook is exported so App.jsx can call fetchTrail via mapRef.showTrail().
 */
import { useState, useCallback } from 'react'
import { Source, Layer } from 'react-map-gl/maplibre'

export function useTrail() {
  const [trail, setTrail] = useState([])

  const fetchTrail = useCallback(async (vid, tid) => {
    try {
      const res = await fetch('/histlocations/', {
        method:  'POST',
        headers: { 'Content-Type': 'application/json' },
        body:    JSON.stringify({ vid, tid }),
      })
      const data = await res.json()
      // GeoJSON coordinates are [longitude, latitude] — note swap from Leaflet [lat, lng]
      const coords = (data.points || []).map(p => [p.lng, p.lat])
      setTrail(coords)
    } catch (e) {
      console.warn('[TrailLayer] fetch failed', e)
      setTrail([])
    }
  }, [])

  const clearTrail = useCallback(() => setTrail([]), [])
  return { trail, fetchTrail, clearTrail }
}

const TRAIL_LAYER = {
  id:   'trail-line',
  type: 'line',
  paint: { 'line-color': '#1D9E75', 'line-width': 3, 'line-opacity': 0.85 },
  layout: { 'line-cap': 'round', 'line-join': 'round' },
}

export default function TrailLayer({ trail }) {
  if (!trail || trail.length < 2) return null

  const geojson = {
    type: 'Feature',
    geometry: { type: 'LineString', coordinates: trail },
    properties: {},
  }

  return (
    <Source id="trail" type="geojson" data={geojson}>
      <Layer {...TRAIL_LAYER} />
    </Source>
  )
}
