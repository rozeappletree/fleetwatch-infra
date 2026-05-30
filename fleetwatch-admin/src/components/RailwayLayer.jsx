/**
 * RailwayLayer.jsx
 * Renders railway_line.geojson and railway_area.geojson as Leaflet GeoJSON layers.
 */
import { GeoJSON, LayerGroup } from 'react-leaflet'
import useGeoJSON from '../hooks/useGeoJSON'
import { railwayLineStyle, railwayAreaStyle, railwayPointToLayer } from '../layers/layerStyles'

function onEachRailwayFeature(feature, layer) {
  const p = feature.properties || {}
  const name = p.name || p.railway || 'Railway feature'
  layer.bindPopup(
    `<div style="font-family:var(--font)">
       <strong style="color:#a78bfa">${name}</strong>
       ${p.railway ? `<div style="color:#64748b;font-size:0.75rem;margin-top:4px">Type: ${p.railway}</div>` : ''}
       ${p.service ? `<div style="color:#64748b;font-size:0.75rem">Service: ${p.service}</div>` : ''}
     </div>`,
    { maxWidth: 200 }
  )
}

export default function RailwayLayer() {
  const lines = useGeoJSON('railway_line.geojson')
  const areas = useGeoJSON('railway_area.geojson')
  const points = useGeoJSON('railway_points.geojson')

  return (
    <LayerGroup>
      {areas.data && (
        <GeoJSON
          key="railway-areas"
          data={areas.data}
          style={railwayAreaStyle}
          onEachFeature={onEachRailwayFeature}
        />
      )}
      {lines.data && (
        <GeoJSON
          key="railway-lines"
          data={lines.data}
          style={railwayLineStyle}
          onEachFeature={onEachRailwayFeature}
        />
      )}
      {points.data && (
        <GeoJSON
          key="railway-points"
          data={points.data}
          pointToLayer={railwayPointToLayer}
          onEachFeature={onEachRailwayFeature}
        />
      )}
    </LayerGroup>
  )
}
