/**
 * TrailLayer.jsx
 * Renders the historical trip trail for a selected truck.
 * Data fetched from /histlocations/ POST endpoint.
 */
import { useState, useCallback } from 'react'
import { Polyline } from 'react-leaflet'

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
      const coords = (data.points || []).map(p => [p.lat, p.lng])
      setTrail(coords)
    } catch (e) {
      console.warn('[TrailLayer] fetch failed', e)
      setTrail([])
    }
  }, [])

  const clearTrail = useCallback(() => setTrail([]), [])

  return { trail, fetchTrail, clearTrail }
}

export default function TrailLayer({ trail }) {
  if (!trail || trail.length < 2) return null

  return (
    <Polyline
      positions={trail}
      pathOptions={{
        color:     '#1D9E75',
        weight:    3,
        opacity:   0.85,
        lineCap:   'round',
        lineJoin:  'round',
        dashArray: null,
      }}
    />
  )
}
