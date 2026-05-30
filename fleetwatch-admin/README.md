# FleetWatch Admin Dashboard

Welcome to the **FleetWatch Admin** frontend repository. This application is a real-time, interactive geospatial dashboard designed to monitor and manage transit and fleet tracking data. It leverages React, Leaflet, and MapLibre to provide high-performance mapping capabilities, integrating point, line, and area infrastructure datasets.

## Features

- **Real-Time Map Visualization**: High-performance mapping using React Leaflet.
- **Rich Geospatial Data Layers**: Support for rendering points, lines, and complex polygons (such as highways and railway systems).
- **Custom Identifiers**: Employs emojis and distinct UI markers (e.g., gates 🚧, road barriers ⛔, and bus stops 🚌) to dynamically map transit infrastructure.
- **Layer Controls**: Easily toggle between points of interest and infrastructure layers using the built-in Leaflet Layer Controls.
- **Production-Ready**: Dockerized for seamless deployment.

## Tech Stack

- **Framework**: [React](https://react.dev/) powered by [Vite](https://vitejs.dev/)
- **Mapping Library**: [Leaflet](https://leafletjs.com/) and `react-leaflet`
- **Data Standard**: GeoJSON
- **Web Server**: Nginx (via Docker)

## Project Structure

```
.
├── public/                 # Static assets, including exported QGIS GeoJSON files
├── src/
│   ├── components/         # Map layers (e.g., HighwayLayer.jsx, RailwayLayer.jsx)
│   ├── layers/             # Layer styling logic (layerStyles.js)
│   ├── App.jsx             # Main application entry point
│   ├── FleetMap.jsx        # Core Leaflet map component
│   └── index.css           # Global map and UI styling
├── nginx.conf              # Nginx server configuration for production builds
├── Dockerfile              # Containerization instructions
└── package.json            # Node.js dependencies and scripts
```

## Setup Instructions

### Local Development

1. Install dependencies:
   ```bash
   npm install
   ```

2. Start the development server:
   ```bash
   npm run dev
   ```

### Production Build

1. Build the frontend assets:
   ```bash
   npm run build
   ```

2. The static files will be generated in the `dist` folder. You can serve them via any standard web server (e.g., Nginx).

### Docker Deployment

To build and run the Dockerized version of the application:

```bash
docker build -t fleetwatch-admin .
docker run -p 80:80 fleetwatch-admin
```

The application will be accessible at `http://localhost`.

## Data Management

All geospatial mapping datasets (`highway_points.geojson`, `highway_area.geojson`, `railway_points.geojson`, etc.) must be placed in the `/public/geojson/` directory so they can be fetched efficiently by the frontend asynchronously.
