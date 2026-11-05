CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS workspaces (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL UNIQUE,
  display_name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS memberships (
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id uuid NOT NULL REFERENCES users(id),
  role text NOT NULL CHECK (role IN ('administrator', 'analyst', 'reader')),
  PRIMARY KEY (workspace_id, user_id)
);

CREATE TABLE IF NOT EXISTS projects (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS projects_workspace_idx ON projects(workspace_id);

CREATE TABLE IF NOT EXISTS areas (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  project_id uuid NOT NULL REFERENCES projects(id),
  name text NOT NULL,
  geometry geometry(MultiPolygon, 4326) NOT NULL,
  area_ha double precision NOT NULL CHECK (area_ha > 0),
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS areas_workspace_idx ON areas(workspace_id);
CREATE INDEX IF NOT EXISTS areas_geometry_gist ON areas USING gist(geometry);

CREATE TABLE IF NOT EXISTS taxonomies (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code text NOT NULL UNIQUE,
  name text NOT NULL,
  source_product text NOT NULL,
  source_url text NOT NULL,
  source_date date NOT NULL,
  version text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS classes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  taxonomy_id uuid NOT NULL REFERENCES taxonomies(id),
  code text NOT NULL,
  name text NOT NULL,
  parent_code text,
  color text NOT NULL,
  technical boolean NOT NULL DEFAULT false,
  UNIQUE (taxonomy_id, code)
);
CREATE TABLE IF NOT EXISTS taxonomy_mappings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  taxonomy_id uuid NOT NULL REFERENCES taxonomies(id),
  source_name text NOT NULL,
  relation text NOT NULL CHECK (relation IN ('exact', 'broader', 'partial', 'none')),
  notes text NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS processing_recipes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code text NOT NULL UNIQUE,
  version text NOT NULL,
  config jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS model_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code text NOT NULL UNIQUE,
  status text NOT NULL CHECK (status IN ('draft', 'approved', 'retired')),
  checkpoint jsonb NOT NULL,
  contract jsonb NOT NULL,
  metrics jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS approvals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  model_version_id uuid NOT NULL REFERENCES model_versions(id),
  approved_by uuid REFERENCES users(id),
  approved_at timestamptz,
  notes text NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS classification_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  project_id uuid NOT NULL REFERENCES projects(id),
  area_id uuid NOT NULL REFERENCES areas(id),
  status text NOT NULL CHECK (status IN ('queued', 'running', 'cancelling', 'cancelled', 'succeeded', 'partial', 'insufficient_data', 'failed')),
  requested_start timestamptz NOT NULL,
  requested_end timestamptz NOT NULL,
  effective_start timestamptz,
  effective_end timestamptz,
  sensor_policy jsonb NOT NULL,
  recipe_id uuid REFERENCES processing_recipes(id),
  model_version_id uuid REFERENCES model_versions(id),
  manifest jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (requested_start < requested_end)
);
CREATE INDEX IF NOT EXISTS runs_workspace_idx ON classification_runs(workspace_id, created_at DESC);

CREATE TABLE IF NOT EXISTS jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL UNIQUE REFERENCES classification_runs(id) ON DELETE CASCADE,
  status text NOT NULL CHECK (status IN ('queued', 'running', 'cancelling', 'cancelled', 'succeeded', 'partial', 'insufficient_data', 'failed')),
  lease_until timestamptz,
  heartbeat_at timestamptz,
  attempts integer NOT NULL DEFAULT 0,
  last_error jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS job_steps (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  name text NOT NULL,
  status text NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'skipped', 'failed')),
  input_refs jsonb NOT NULL DEFAULT '{}'::jsonb,
  output_refs jsonb NOT NULL DEFAULT '{}'::jsonb,
  attempts integer NOT NULL DEFAULT 0,
  started_at timestamptz,
  finished_at timestamptz,
  error jsonb,
  UNIQUE(job_id, name)
);
CREATE TABLE IF NOT EXISTS job_events (
  id bigserial PRIMARY KEY,
  job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  event_type text NOT NULL,
  message text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS scene_assets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider text NOT NULL,
  collection text NOT NULL,
  scene_id text NOT NULL,
  observed_at timestamptz NOT NULL,
  cloud_cover double precision,
  metadata jsonb NOT NULL,
  UNIQUE(provider, collection, scene_id)
);
CREATE TABLE IF NOT EXISTS run_scene_assets (
  run_id uuid NOT NULL REFERENCES classification_runs(id) ON DELETE CASCADE,
  scene_asset_id uuid NOT NULL REFERENCES scene_assets(id),
  role text NOT NULL CHECK (role IN ('optical', 'radar')),
  PRIMARY KEY(run_id, scene_asset_id)
);
CREATE TABLE IF NOT EXISTS artifacts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES classification_runs(id) ON DELETE CASCADE,
  kind text NOT NULL,
  object_key text NOT NULL,
  checksum text NOT NULL,
  size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
  crs text,
  resolution_m double precision,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  UNIQUE(run_id, kind)
);

CREATE TABLE IF NOT EXISTS sample_sets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  project_id uuid NOT NULL REFERENCES projects(id),
  name text NOT NULL,
  status text NOT NULL DEFAULT 'draft',
  source jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS samples (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sample_set_id uuid NOT NULL REFERENCES sample_sets(id) ON DELETE CASCADE,
  geometry geometry(Geometry, 4326) NOT NULL,
  class_code text NOT NULL,
  source text NOT NULL,
  observation_start timestamptz,
  observation_end timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS samples_geometry_gist ON samples USING gist(geometry);
CREATE TABLE IF NOT EXISTS sample_revisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sample_id uuid NOT NULL REFERENCES samples(id) ON DELETE CASCADE,
  revision integer NOT NULL,
  class_code text NOT NULL,
  author uuid REFERENCES users(id),
  justification text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(sample_id, revision)
);
CREATE TABLE IF NOT EXISTS dataset_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sample_set_id uuid NOT NULL REFERENCES sample_sets(id),
  split_policy jsonb NOT NULL,
  artifact_key text NOT NULL,
  checksum text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS evaluations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  model_version_id uuid NOT NULL REFERENCES model_versions(id),
  dataset_version_id uuid REFERENCES dataset_versions(id),
  metrics jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS class_statistics (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES classification_runs(id) ON DELETE CASCADE,
  class_code text NOT NULL,
  area_ha double precision NOT NULL,
  percent double precision NOT NULL,
  pixels bigint NOT NULL,
  UNIQUE(run_id, class_code)
);
CREATE TABLE IF NOT EXISTS review_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES classification_runs(id),
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  status text NOT NULL DEFAULT 'open',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS corrections (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  review_session_id uuid NOT NULL REFERENCES review_sessions(id),
  geometry geometry(Geometry, 4326) NOT NULL,
  original_class text NOT NULL,
  corrected_class text NOT NULL,
  author uuid REFERENCES users(id),
  justification text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS audit_events (
  id bigserial PRIMARY KEY,
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  user_id uuid REFERENCES users(id),
  action text NOT NULL,
  entity_type text NOT NULL,
  entity_id uuid,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO workspaces(id, name) VALUES ('00000000-0000-0000-0000-000000000001', 'Workspace local') ON CONFLICT DO NOTHING;
INSERT INTO taxonomies(code, name, source_product, source_url, source_date, version)
VALUES ('operational_broad_v1', 'Cobertura recente ampla', 'Legenda operacional própria; relação auxiliar ao MapBiomas', 'https://brasil.mapbiomas.org/downloads/codigos-de-legenda/', '2026-10-23', '1.0.0')
ON CONFLICT (code) DO NOTHING;
INSERT INTO classes(taxonomy_id, code, name, parent_code, color, technical)
SELECT t.id, c.code, c.name, c.parent_code, c.color, c.technical
FROM taxonomies t CROSS JOIN (VALUES
  ('native_vegetation','Vegetação nativa',NULL,'#16803c',false),
  ('pasture','Pastagem',NULL,'#c8a951',false),
  ('agriculture','Agricultura',NULL,'#e07b39',false),
  ('silviculture','Silvicultura',NULL,'#6b4f2a',false),
  ('bare_soil','Solo exposto',NULL,'#d8b07a',false),
  ('water','Água',NULL,'#2d7dd2',false),
  ('built','Área construída',NULL,'#b23a48',false),
  ('other_natural','Outras naturais não vegetadas',NULL,'#9b9b9b',false),
  ('undetermined','Não determinado',NULL,'#6d6875',true),
  ('insufficient_data','Dados insuficientes',NULL,'#343a40',true),
  ('nodata','Sem dados / fora do polígono',NULL,'#000000',true)
) AS c(code,name,parent_code,color,technical)
WHERE t.code = 'operational_broad_v1'
ON CONFLICT (taxonomy_id, code) DO NOTHING;
