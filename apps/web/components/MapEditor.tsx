'use client';

import { useMemo } from 'react';
import { MapContainer, Polygon, TileLayer, useMapEvents } from 'react-leaflet';
import type { LatLngExpression } from 'leaflet';

type Props = { points: [number, number][]; onChange: (points: [number, number][]) => void };

function Capture({ onClick }: { onClick: (point: [number, number]) => void }) {
  useMapEvents({ click(event) { onClick([event.latlng.lat, event.latlng.lng]); } });
  return null;
}

export default function MapEditor({ points, onChange }: Props) {
  const polygon = useMemo(() => points.map(([lat, lng]) => [lat, lng] as LatLngExpression), [points]);
  return <div className="map-wrap">
    <MapContainer center={[-15.8, -47.9]} zoom={4} scrollWheelZoom className="map">
      <TileLayer attribution="&copy; OpenStreetMap contributors" url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png" />
      <Capture onClick={(point) => onChange([...points, point])} />
      {polygon.length >= 3 && <Polygon positions={polygon} pathOptions={{ color: '#167d63', fillOpacity: 0.25 }} />}
    </MapContainer>
  </div>;
}
