/**
 * layerStyles.js
 * Centralized styling for all GeoJSON/Leaflet map layers.
 * Each style is a function so it can be used as a Leaflet style callback.
 */
import L from 'leaflet'

// ── Railway Lines ──────────────────────────────────────────────────────────
export function railwayLineStyle(feature) {
  const service = feature?.properties?.service
  return {
    color:    service ? '#9333ea' : '#7e22ce',  // deep purple
    weight:   service ? 2.5 : 4.5,
    opacity:  1.0,
    dashArray: service ? '5 5' : '10 8', 
    lineCap:  'round',
    lineJoin: 'round',
  }
}

// ── Railway Areas (platforms, yards) ──────────────────────────────────────
export function railwayAreaStyle() {
  return {
    color:       '#7e22ce',
    weight:      2.5,
    opacity:     1.0,
    fillColor:   '#581c87',
    fillOpacity: 0.4,
  }
}

// ── Railway Points (stations, crossings) ──────────────────────────────────
export function railwayPointToLayer(feature, latlng) {
  const isCrossing = feature?.properties?.railway === 'level_crossing';
  
  if (isCrossing) {
    const icon = L.divIcon({
      html: '<div style="font-size: 16px; background: white; border-radius: 50%; width: 20px; height: 20px; display: flex; align-items: center; justify-content: center; box-shadow: 0 2px 4px rgba(0,0,0,0.4);">🚧</div>',
      className: '',
      iconSize: [20, 20],
      iconAnchor: [10, 10]
    });
    return L.marker(latlng, { icon });
  }

  return L.circleMarker(latlng, {
    radius: 4.5,
    fillColor: '#fde047', // Standard bright yellow for transit stops
    color: '#000000',     // Stark black border for high contrast
    weight: 2,
    opacity: 1,
    fillOpacity: 1
  })
}

// ── Highway Lines ──────────────────────────────────────────────────────────
export function highwayLineStyle(feature) {
  const hw = feature?.properties?.highway
  const palette = {
    motorway:       { color: '#be123c', weight: 6, opacity: 1.0 }, // dark rose/red
    motorway_link:  { color: '#be123c', weight: 4, opacity: 1.0 },
    trunk:          { color: '#c2410c', weight: 5, opacity: 1.0 }, // dark orange
    trunk_link:     { color: '#c2410c', weight: 3, opacity: 1.0 },
    primary:        { color: '#a16207', weight: 4.5, opacity: 1.0 }, // dark yellow/amber
    primary_link:   { color: '#a16207', weight: 3, opacity: 1.0 },
    secondary:      { color: '#15803d', weight: 4, opacity: 1.0 }, // dark green
    secondary_link: { color: '#15803d', weight: 2.5, opacity: 1.0 },
    tertiary:       { color: '#0f766e', weight: 3.5, opacity: 1.0 }, // dark teal
    tertiary_link:  { color: '#0f766e', weight: 2, opacity: 1.0 },
    residential:    { color: '#0369a1', weight: 3, opacity: 1.0 }, // dark sky blue
    service:        { color: '#334155', weight: 2, opacity: 0.9 }, // dark slate
    track:          { color: '#44403c', weight: 2, opacity: 0.9, dashArray: '4 4' },
    path:           { color: '#57534e', weight: 2, opacity: 0.9, dashArray: '2 4' },
  }
  const s = palette[hw] || { color: '#334155', weight: 2, opacity: 0.8 }
  return { ...s, lineCap: 'round', lineJoin: 'round' }
}

// ── Highway Areas (parking, yards) ────────────────────────────────────────
export function highwayAreaStyle() {
  return {
    color:       '#334155',
    weight:      2,
    opacity:     0.8,
    fillColor:   '#475569',
    fillOpacity: 0.3,
  }
}

// ── Highway Points (signals, stops) ───────────────────────────────────────
export function highwayPointToLayer(feature, latlng) {
  const props = feature?.properties || {};
  
  let emoji = null;
  if (props.barrier === 'gate') emoji = '⛔';
  else if (props.highway === 'bus_stop') emoji = '🚌';
  else if (props.highway === 'traffic_signals') emoji = '🚥';
  else if (props.railway === 'level_crossing') emoji = '🚧'; // sometimes in highway layer
  
  if (emoji) {
    const icon = L.divIcon({
      html: `<div style="font-size: 14px; background: white; border-radius: 50%; width: 22px; height: 22px; display: flex; align-items: center; justify-content: center; box-shadow: 0 2px 4px rgba(0,0,0,0.4); border: 1px solid #cbd5e1;">${emoji}</div>`,
      className: '',
      iconSize: [22, 22],
      iconAnchor: [11, 11]
    });
    return L.marker(latlng, { icon });
  }

  // Fallback for generic nodes
  return L.circleMarker(latlng, {
    radius: 4,
    fillColor: '#ef4444', // Standard bright red for road signals/stops
    color: '#ffffff',     // Clean white border
    weight: 2,
    opacity: 1,
    fillOpacity: 1
  })
}

// ── Truck / Vehicle markers ─────────────────────────────────────────────────
export const TRUCK_STATUS_COLORS = {
  ok:        '#1D9E75',
  deviation: '#E24B4A',
  stopped:   '#EF9F27',
}
