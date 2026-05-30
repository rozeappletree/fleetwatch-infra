/**
 * useGeoJSON.js
 * Fetches a GeoJSON file from /geojson/{name}.geojson (served from public/).
 * Returns { data, loading, error }.
 */
import { useState, useEffect } from 'react'

export default function useGeoJSON(filename) {
  const [data, setData]       = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError]     = useState(null)

  useEffect(() => {
    if (!filename) return
    setLoading(true)
    setError(null)

    fetch(`/geojson/${filename}`)
      .then(r => {
        if (!r.ok) throw new Error(`HTTP ${r.status} loading ${filename}`)
        return r.json()
      })
      .then(json => { setData(json); setLoading(false) })
      .catch(err => { console.error('[useGeoJSON]', err); setError(err.message); setLoading(false) })
  }, [filename])

  return { data, loading, error }
}
