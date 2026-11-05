export type RunStatus = 'queued' | 'running' | 'cancelling' | 'cancelled' | 'succeeded' | 'partial' | 'insufficient_data' | 'failed';
export interface GeoJSONGeometry { type: 'Polygon' | 'MultiPolygon'; coordinates: number[][][] | number[][][][]; }
export interface Area { id: string; projectId: string; name: string; areaHa: number; version: number; geometry: GeoJSONGeometry; }
export interface Scene { id: string; collection: string; observedAt: string; cloudCover?: number; assets: Record<string, string>; }
export interface Run { id: string; projectId: string; areaId: string; status: RunStatus; requestedStart: string; requestedEnd: string; effectiveStart?: string; effectiveEnd?: string; steps?: Array<{ name: string; status: string; attempts: number; error?: unknown }>; manifest?: Record<string, unknown>; }
