/**
 * HighwayLayer.jsx
 * Renders highway_line.geojson as a styled Leaflet GeoJSON layer.
 * Large file (~70MB) — loaded async; shows nothing while loading.
 */
import { GeoJSON, LayerGroup } from 'react-leaflet'
import useGeoJSON from '../hooks/useGeoJSON'
import { highwayLineStyle, highwayAreaStyle, highwayPointToLayer } from '../layers/layerStyles'

function onEachHighwayFeature(feature, layer) {
  const p = feature.properties || {}
  const name = p.name || p.ref || p.highway || 'Road'
  layer.bindPopup(
    `<div style="font-family:var(--font)">
       <strong style="color:#fb923c">${name}</strong>
       ${p.highway ? `<div style="color:#64748b;font-size:0.75rem;margin-top:4px">Class: ${p.highway}</div>` : ''}
       ${p.ref ? `<div style="color:#64748b;font-size:0.75rem">Ref: ${p.ref}</div>` : ''}
       ${p.maxspeed ? `<div style="color:#64748b;font-size:0.75rem">Speed limit: ${p.maxspeed}</div>` : ''}
     </div>`,
    { maxWidth: 200 }
  )
}

export default function HighwayLayer() {
  const lines = useGeoJSON('highway_line.geojson')
  const areas = useGeoJSON('highway_area.geojson')
  const points = useGeoJSON('highway_points.geojson')

  return (
    <LayerGroup>
      {areas.data && (
        <GeoJSON
          key="highway-areas"
          data={areas.data}
          style={highwayAreaStyle}
          onEachFeature={onEachHighwayFeature}
        />
      )}
      {lines.data && (
        <GeoJSON
          key="highway-lines"
          data={lines.data}
          style={highwayLineStyle}
          onEachFeature={onEachHighwayFeature}
        />
      )}
      {points.data && (
        <GeoJSON
          key="highway-points"
          data={points.data}
          pointToLayer={highwayPointToLayer}
          onEachFeature={onEachHighwayFeature}
        />
      )}
    </LayerGroup>
  )
}
