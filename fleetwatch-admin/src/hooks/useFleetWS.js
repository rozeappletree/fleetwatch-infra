/**
 * useFleetWS.js
 * WebSocket hook — connects to /locations/, auto-reconnects with exponential
 * backoff (1 s → 2 s → 4 s → … → 30 s), calls onEvent on every truck message.
 */
import { useEffect, useRef } from 'react'

const VIZAG_BOUNDS = { lngMin: 83.270, lngMax: 83.320, latMin: 17.690, latMax: 17.730 }

export function checkVizagBounds(lng, lat) {
  const ok =
    lng >= VIZAG_BOUNDS.lngMin && lng <= VIZAG_BOUNDS.lngMax &&
    lat >= VIZAG_BOUNDS.latMin && lat <= VIZAG_BOUNDS.latMax
  if (!ok) console.warn(`[bounds] [${lng},${lat}] outside Vizag port bounds`)
  return ok
}

export default function useFleetWS({ onEvent, onRoute, onStatusChange }) {
  const wsRef    = useRef(null)
  const delay    = useRef(1000)
  const timerRef = useRef(null)

  useEffect(() => {
    let cancelled = false

    function connect() {
      const proto = location.protocol === 'https:' ? 'wss' : 'ws'
      const ws = new WebSocket(`${proto}://${location.host}/locations/`)
      wsRef.current = ws

      ws.onopen = () => {
        if (cancelled) { ws.close(); return }
        delay.current = 1000
        onStatusChange?.(true)
      }

      ws.onmessage = (e) => {
        if (cancelled) return
        try {
          const raw = JSON.parse(e.data)
          if (raw.type === 'route') { onRoute?.(raw); return }
          const ev = raw.VP ?? raw
          if (ev?.lat != null && ev?.lng != null) onEvent?.(ev)
        } catch { /* ignore parse errors */ }
      }

      ws.onclose = ws.onerror = () => {
        if (cancelled) return
        onStatusChange?.(false)
        timerRef.current = setTimeout(() => {
          delay.current = Math.min(delay.current * 2, 30000)
          connect()
        }, delay.current)
      }
    }

    connect()
    return () => {
      cancelled = true
      clearTimeout(timerRef.current)
      wsRef.current?.close()
    }
  }, []) // eslint-disable-line react-hooks/exhaustive-deps
}
