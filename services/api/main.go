package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultWorkspace = "00000000-0000-0000-0000-000000000001"

type server struct {
	db      *pgxpool.Pool
	stacURL string
}

type projectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type areaRequest struct {
	ProjectID string          `json:"projectId"`
	Name      string          `json:"name"`
	Geometry  json.RawMessage `json:"geometry"`
}

type runRequest struct {
	ProjectID string   `json:"projectId"`
	AreaID    string   `json:"areaId"`
	Start     string   `json:"start"`
	End       string   `json:"end"`
	Sensors   []string `json:"sensors"`
}

type stacFeature struct {
	ID         string                 `json:"id"`
	Collection string                 `json:"collection"`
	Properties map[string]interface{} `json:"properties"`
	Assets     map[string]struct {
		Href  string `json:"href"`
		Title string `json:"title"`
		Type  string `json:"type"`
	} `json:"assets"`
}

type stacResponse struct {
	Features []stacFeature `json:"features"`
}

func main() {
	ctx := context.Background()
	databaseURL := env("DATABASE_URL", "postgres://earth:earth@localhost:5432/earth_classifier?sslmode=disable")
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		panic(err)
	}
	s := &server{db: db, stacURL: env("STAC_API_URL", "https://planetarycomputer.microsoft.com/api/stac/v1")}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.health)
	mux.HandleFunc("/api/v1/projects", s.projects)
	mux.HandleFunc("/api/v1/areas", s.areas)
	mux.HandleFunc("/api/v1/availability", s.availability)
	mux.HandleFunc("/api/v1/runs", s.runs)
	mux.HandleFunc("/api/v1/runs/", s.runs)
	mux.HandleFunc("/api/v1/models", s.models)
	mux.HandleFunc("/api/v1/taxonomies", s.taxonomies)
	mux.HandleFunc("/api/v1/sample-sets", s.sampleSets)
	mux.HandleFunc("/api/v1/sample-sets/", s.sampleSets)
	h := cors(withWorkspace(mux))
	port := env("API_PORT", "8080")
	slog.Info("api listening", "port", port)
	if err := http.ListenAndServe(":"+port, h); err != nil {
		panic(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func withWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workspace := r.Header.Get("X-Workspace-ID")
		if workspace == "" {
			workspace = defaultWorkspace
		}
		if len(workspace) != 36 {
			writeError(w, http.StatusUnauthorized, "workspace inválido")
			return
		}
		ctx := context.WithValue(r.Context(), workspaceKey{}, workspace)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type workspaceKey struct{}

func workspace(r *http.Request) string {
	value, _ := r.Context().Value(workspaceKey{}).(string)
	return value
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", env("WEB_ORIGIN", "http://localhost:3000"))
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Workspace-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.db.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "banco indisponível")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) projects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := s.db.Query(r.Context(), `SELECT id, name, description, created_at FROM projects WHERE workspace_id=$1 ORDER BY created_at DESC`, workspace(r))
		if err != nil { writeError(w, 500, err.Error()); return }
		defer rows.Close()
		items := []map[string]interface{}{}
		for rows.Next() {
			var id, name, description string; var created time.Time
			if err := rows.Scan(&id, &name, &description, &created); err != nil { writeError(w, 500, err.Error()); return }
			items = append(items, map[string]interface{}{"id": id, "name": name, "description": description, "createdAt": created})
		}
		writeJSON(w, 200, items)
	case http.MethodPost:
		var req projectRequest
		if !decode(w, r, &req) || strings.TrimSpace(req.Name) == "" { return }
		var id string
		err := s.db.QueryRow(r.Context(), `INSERT INTO projects(workspace_id,name,description) VALUES($1,$2,$3) RETURNING id`, workspace(r), strings.TrimSpace(req.Name), req.Description).Scan(&id)
		if err != nil { writeError(w, 500, err.Error()); return }
		writeJSON(w, 201, map[string]string{"id": id, "name": req.Name})
	default: writeError(w, 405, "método não permitido")
	}
}

func (s *server) areas(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		projectID := r.URL.Query().Get("projectId")
		rows, err := s.db.Query(r.Context(), `SELECT id, project_id, name, area_ha, version, ST_AsGeoJSON(geometry) FROM areas WHERE workspace_id=$1 AND ($2='' OR project_id=$2) ORDER BY created_at DESC`, workspace(r), projectID)
		if err != nil { writeError(w, 500, err.Error()); return }
		defer rows.Close(); items := []map[string]interface{}{}
		for rows.Next() {
			var id, project, name, geo string; var hectares float64; var version int
			if err := rows.Scan(&id, &project, &name, &hectares, &version, &geo); err != nil { writeError(w, 500, err.Error()); return }
			var geometry interface{}; _ = json.Unmarshal([]byte(geo), &geometry)
			items = append(items, map[string]interface{}{"id": id, "projectId": project, "name": name, "areaHa": hectares, "version": version, "geometry": geometry})
		}
		writeJSON(w, 200, items); return
	}
	if r.Method != http.MethodPost { writeError(w, 405, "método não permitido"); return }
	var req areaRequest
	if !decode(w, r, &req) || strings.TrimSpace(req.Name) == "" || len(req.Geometry) == 0 { return }
	if err := validateGeoJSON(req.Geometry); err != nil { writeError(w, 400, err.Error()); return }
	var id string; var hectares float64
	err := s.db.QueryRow(r.Context(), `WITH input AS (SELECT ST_Multi(ST_SetSRID(ST_GeomFromGeoJSON($1),4326)) AS geom), valid AS (SELECT geom FROM input WHERE ST_IsValid(geom) AND NOT ST_IsEmpty(geom)) INSERT INTO areas(workspace_id,project_id,name,geometry,area_ha) SELECT $2,p.id,$4,valid.geom,ST_Area(ST_Transform(valid.geom,6933))/10000 FROM valid JOIN projects p ON p.id=$3 AND p.workspace_id=$2 RETURNING areas.id, areas.area_ha`, string(req.Geometry), workspace(r), req.ProjectID, strings.TrimSpace(req.Name)).Scan(&id, &hectares)
	if errors.Is(err, pgx.ErrNoRows) { writeError(w, 400, "geometria inválida, vazia ou não suportada"); return }
	if err != nil { writeError(w, 500, err.Error()); return }
	writeJSON(w, 201, map[string]interface{}{"id": id, "areaHa": hectares, "geometryCrs": "EPSG:4326"})
}

func validateGeoJSON(raw []byte) error {
	var value struct { Type string `json:"type"`; Coordinates interface{} `json:"coordinates"` }
	if err := json.Unmarshal(raw, &value); err != nil { return fmt.Errorf("GeoJSON inválido: %w", err) }
	if value.Type != "Polygon" && value.Type != "MultiPolygon" { return fmt.Errorf("use Polygon ou MultiPolygon") }
	if value.Coordinates == nil { return fmt.Errorf("coordenadas ausentes") }
	return nil
}

func validateSampleGeoJSON(raw []byte) error {
	var value struct { Type string `json:"type"`; Coordinates interface{} `json:"coordinates"` }
	if err := json.Unmarshal(raw, &value); err != nil { return fmt.Errorf("GeoJSON inválido: %w", err) }
	if value.Type != "Point" && value.Type != "Polygon" && value.Type != "MultiPolygon" { return fmt.Errorf("amostra deve ser Point, Polygon ou MultiPolygon") }
	if value.Coordinates == nil { return fmt.Errorf("coordenadas ausentes") }
	return nil
}

func (s *server) availability(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { writeError(w, 405, "método não permitido"); return }
	var req struct { Geometry json.RawMessage `json:"geometry"`; Start string `json:"start"`; End string `json:"end"` }
	if !decode(w, r, &req) || len(req.Geometry) == 0 { return }
	if err := validatePeriod(req.Start, req.End); err != nil { writeError(w, 400, err.Error()); return }
	if err := validateGeoJSON(req.Geometry); err != nil { writeError(w, 400, err.Error()); return }
	all, err := s.searchScenes(r.Context(), req.Geometry, req.Start, req.End)
	if err != nil { writeError(w, 502, "provedor STAC indisponível: "+err.Error()); return }
	writeJSON(w, 200, map[string]interface{}{"requestedStart": req.Start, "requestedEnd": req.End, "scenes": all, "sceneCount": len(all), "note": "cobertura de cena é filtro inicial; máscara por pixel será calculada no processamento"})
}

func (s *server) searchScenes(ctx context.Context, geometry json.RawMessage, start, end string) ([]map[string]interface{}, error) {
	collections := []string{"sentinel-2-l2a", "sentinel-1-rtc"}
	result := []map[string]interface{}{}
	for _, collection := range collections {
		query := map[string]interface{}{"collections": []string{collection}, "datetime": start + "/" + end, "intersects": json.RawMessage(geometry), "limit": 100}
		if collection == "sentinel-2-l2a" { query["query"] = map[string]interface{}{"eo:cloud_cover": map[string]interface{}{"lt": 80}} }
		body, _ := json.Marshal(query)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, s.stacURL+"/search", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req); if err != nil { return nil, err }
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 20<<20)); resp.Body.Close()
		if readErr != nil { return nil, readErr }; if resp.StatusCode >= 300 { return nil, fmt.Errorf("HTTP %d", resp.StatusCode) }
		var parsed stacResponse; if err := json.Unmarshal(data, &parsed); err != nil { return nil, err }
		for _, feature := range parsed.Features {
			observed := fmt.Sprint(feature.Properties["datetime"])
			entry := map[string]interface{}{"id": feature.ID, "collection": collection, "observedAt": observed, "cloudCover": feature.Properties["eo:cloud_cover"], "assets": map[string]string{}}
			assets := entry["assets"].(map[string]string)
			for _, key := range []string{"B02", "B03", "B04", "B08", "B05", "B06", "B07", "B8A", "B11", "B12", "B01", "B09", "SCL", "vv", "vh"} { if asset, ok := feature.Assets[key]; ok { assets[key] = asset.Href } }
			result = append(result, entry)
		}
	}
	return result, nil
}

func (s *server) runs(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/runs")
	if path == "" || path == "/" {
		if r.Method == http.MethodGet { s.listRuns(w, r); return }
		if r.Method == http.MethodPost { s.createRun(w, r); return }
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 1 && parts[0] != "" {
		if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost { s.cancelRun(w, r, parts[0]); return }
		if len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodGet { s.runEvents(w, r, parts[0]); return }
		if len(parts) == 1 && r.Method == http.MethodGet { s.runDetail(w, r, parts[0]); return }
	}
	writeError(w, 404, "execução não encontrada")
}

func (s *server) listRuns(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(), `SELECT id, project_id, area_id, status, requested_start, requested_end, effective_start, effective_end, manifest, created_at FROM classification_runs WHERE workspace_id=$1 ORDER BY created_at DESC LIMIT 100`, workspace(r))
	if err != nil { writeError(w, 500, err.Error()); return }; defer rows.Close(); items := []map[string]interface{}{}
	for rows.Next() { var id, project, area, status string; var start, end time.Time; var effectiveStart, effectiveEnd *time.Time; var manifest []byte; var created time.Time; if err := rows.Scan(&id,&project,&area,&status,&start,&end,&effectiveStart,&effectiveEnd,&manifest,&created); err != nil { writeError(w,500,err.Error()); return }; var m interface{}; _=json.Unmarshal(manifest,&m); items=append(items,map[string]interface{}{"id":id,"projectId":project,"areaId":area,"status":status,"requestedStart":start,"requestedEnd":end,"effectiveStart":effectiveStart,"effectiveEnd":effectiveEnd,"manifest":m,"createdAt":created}) }
	writeJSON(w, 200, items)
}

func (s *server) createRun(w http.ResponseWriter, r *http.Request) {
	var req runRequest; if !decode(w,r,&req) { return }; if err:=validatePeriod(req.Start,req.End); err!=nil { writeError(w,400,err.Error()); return }; if req.ProjectID=="" || req.AreaID=="" { writeError(w,400,"projectId e areaId são obrigatórios"); return }
	policy := map[string]interface{}{"sensors": req.Sensors, "interval": "[start,end]", "windowDays": daysBetween(req.Start, req.End), "modelRequired": true}
	policyJSON,_:=json.Marshal(policy); var id string
	tx, err := s.db.Begin(r.Context())
	if err == nil {
		defer tx.Rollback(r.Context())
		var exists int
		err = tx.QueryRow(r.Context(), `SELECT 1 FROM projects p JOIN areas a ON a.project_id=p.id AND a.workspace_id=p.workspace_id WHERE p.id=$1 AND a.id=$2 AND p.workspace_id=$3`, req.ProjectID, req.AreaID, workspace(r)).Scan(&exists)
		if err != nil { err = fmt.Errorf("projeto ou área não pertence ao workspace") }
		var jobID string
		if err == nil { err = tx.QueryRow(r.Context(), `INSERT INTO classification_runs(workspace_id,project_id,area_id,status,requested_start,requested_end,sensor_policy) VALUES($1,$2,$3,'queued',$4,$5,$6) RETURNING id`, workspace(r), req.ProjectID, req.AreaID, timeValue(req.Start), timeValue(req.End), policyJSON).Scan(&id) }
		if err == nil { err = tx.QueryRow(r.Context(), `INSERT INTO jobs(run_id,status) VALUES($1,'queued') RETURNING id`, id).Scan(&jobID) }
		for _, step := range []string{"validate", "availability", "materialize", "embeddings", "classify", "quality", "publish"} { if err == nil { _, err = tx.Exec(r.Context(), `INSERT INTO job_steps(job_id,name,status) VALUES($1,$2,'queued')`, jobID, step) } }
		if err == nil { _, err = tx.Exec(r.Context(), `INSERT INTO job_events(job_id,event_type,message) VALUES($1,'queued','Execução criada; aguardando o worker')`, jobID) }
		if err == nil { err = tx.Commit(r.Context()) }
	}
	if err!=nil { writeError(w,500,err.Error()); return }; writeJSON(w,202,map[string]interface{}{"id":id,"status":"queued","message":"job assíncrono criado; acompanhe por polling"})
}

func (s *server) runDetail(w http.ResponseWriter, r *http.Request, id string) { var project,area,status string; var start,end time.Time; var effectiveStart,effectiveEnd *time.Time; var manifest []byte; err:=s.db.QueryRow(r.Context(),`SELECT project_id,area_id,status,requested_start,requested_end,effective_start,effective_end,manifest FROM classification_runs WHERE id=$1 AND workspace_id=$2`,id,workspace(r)).Scan(&project,&area,&status,&start,&end,&effectiveStart,&effectiveEnd,&manifest); if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"execução não encontrada");return};if err!=nil{writeError(w,500,err.Error());return}; var m interface{};_=json.Unmarshal(manifest,&m); var steps []map[string]interface{}; rows,err:=s.db.Query(r.Context(),`SELECT js.name,js.status,js.attempts,js.started_at,js.finished_at,js.error FROM job_steps js JOIN jobs j ON j.id=js.job_id WHERE j.run_id=$1 ORDER BY js.id`,id);if err==nil{defer rows.Close();for rows.Next(){var name,st string;var attempts int;var began,finished *time.Time;var e []byte;_=rows.Scan(&name,&st,&attempts,&began,&finished,&e);var em interface{};_=json.Unmarshal(e,&em);steps=append(steps,map[string]interface{}{"name":name,"status":st,"attempts":attempts,"startedAt":began,"finishedAt":finished,"error":em})}};writeJSON(w,200,map[string]interface{}{"id":id,"projectId":project,"areaId":area,"status":status,"requestedStart":start,"requestedEnd":end,"effectiveStart":effectiveStart,"effectiveEnd":effectiveEnd,"manifest":m,"steps":steps}) }

func (s *server) runEvents(w http.ResponseWriter,r *http.Request,id string){rows,err:=s.db.Query(r.Context(),`SELECT event_type,message,payload,created_at FROM job_events e JOIN jobs j ON j.id=e.job_id JOIN classification_runs cr ON cr.id=j.run_id WHERE cr.id=$1 AND cr.workspace_id=$2 ORDER BY e.id`,id,workspace(r));if err!=nil{writeError(w,500,err.Error());return};defer rows.Close();items:=[]map[string]interface{}{};for rows.Next(){var t,m string;var p []byte;var at time.Time;_=rows.Scan(&t,&m,&p,&at);var payload interface{};_=json.Unmarshal(p,&payload);items=append(items,map[string]interface{}{"type":t,"message":m,"payload":payload,"createdAt":at})};writeJSON(w,200,items)}
func (s *server) cancelRun(w http.ResponseWriter,r *http.Request,id string){tag,err:=s.db.Exec(r.Context(),`UPDATE classification_runs SET status='cancelling',updated_at=now() WHERE id=$1 AND workspace_id=$2 AND status IN ('queued','running')`,id,workspace(r));if err!=nil{writeError(w,500,err.Error());return};if tag.RowsAffected()==0{writeError(w,409,"execução não pode ser cancelada");return};_,_=s.db.Exec(r.Context(),`UPDATE jobs SET status='cancelling' WHERE run_id=$1`,id);writeJSON(w,202,map[string]string{"status":"cancelling"})}

func (s *server) models(w http.ResponseWriter,r *http.Request){rows,err:=s.db.Query(r.Context(),`SELECT id,code,status,checkpoint,contract,metrics FROM model_versions ORDER BY created_at DESC`);if err!=nil{writeError(w,500,err.Error());return};defer rows.Close();items:=[]map[string]interface{}{};for rows.Next(){var id,code,status string;var checkpoint,contract,metrics []byte;_=rows.Scan(&id,&code,&status,&checkpoint,&contract,&metrics);var c,ct,m interface{};_=json.Unmarshal(checkpoint,&c);_=json.Unmarshal(contract,&ct);_=json.Unmarshal(metrics,&m);items=append(items,map[string]interface{}{"id":id,"code":code,"status":status,"checkpoint":c,"contract":ct,"metrics":m})};writeJSON(w,200,items)}
func (s *server) taxonomies(w http.ResponseWriter,r *http.Request){rows,err:=s.db.Query(r.Context(),`SELECT t.code,t.name,t.version,c.code,c.name,c.color,c.technical FROM taxonomies t JOIN classes c ON c.taxonomy_id=t.id ORDER BY t.code,c.code`);if err!=nil{writeError(w,500,err.Error());return};defer rows.Close();out:=map[string]map[string]interface{}{};for rows.Next(){var tc,tn,tv,cc,cn,color string;var technical bool;_=rows.Scan(&tc,&tn,&tv,&cc,&cn,&color,&technical);if out[tc]==nil{out[tc]=map[string]interface{}{"code":tc,"name":tn,"version":tv,"classes":[]map[string]interface{}{}}};classes:=out[tc]["classes"].([]map[string]interface{});out[tc]["classes"]=append(classes,map[string]interface{}{"code":cc,"name":cn,"color":color,"technical":technical})};values:=[]map[string]interface{}{};for _,v:=range out{values=append(values,v)};writeJSON(w,200,values)}
func (s *server) sampleSets(w http.ResponseWriter,r *http.Request){
	path:=strings.TrimPrefix(r.URL.Path,"/api/v1/sample-sets")
	if path==""||path=="/"{if r.Method==http.MethodGet{rows,err:=s.db.Query(r.Context(),`SELECT id,project_id,name,status,created_at FROM sample_sets WHERE workspace_id=$1 ORDER BY created_at DESC`,workspace(r));if err!=nil{writeError(w,500,err.Error());return};defer rows.Close();items:=[]map[string]interface{}{};for rows.Next(){var id,project,name,status string;var created time.Time;_=rows.Scan(&id,&project,&name,&status,&created);items=append(items,map[string]interface{}{"id":id,"projectId":project,"name":name,"status":status,"createdAt":created})};writeJSON(w,200,items);return};if r.Method!=http.MethodPost{writeError(w,405,"método não permitido");return};var req struct{Name,ProjectID string};if !decode(w,r,&req){return};if req.Name==""||req.ProjectID==""{writeError(w,400,"name e projectId são obrigatórios");return};var id string;err:=s.db.QueryRow(r.Context(),`INSERT INTO sample_sets(workspace_id,project_id,name) SELECT $1,id,$3 FROM projects WHERE id=$2 AND workspace_id=$1 RETURNING sample_sets.id`,workspace(r),req.ProjectID,req.Name).Scan(&id);if err!=nil{if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"projeto não encontrado");return};writeError(w,500,err.Error());return};writeJSON(w,201,map[string]string{"id":id,"status":"draft"});return}
	parts:=strings.Split(strings.Trim(path,"/"),"/");if len(parts)!=2{writeError(w,404,"conjunto de amostras não encontrado");return};setID,action:=parts[0],parts[1]
	if action=="samples"&&r.Method==http.MethodPost{var req struct{Geometry json.RawMessage `json:"geometry"`;ClassCode string `json:"classCode"`;Source string `json:"source"`;ObservationStart string `json:"observationStart"`;ObservationEnd string `json:"observationEnd"`};if !decode(w,r,&req){return};if err:=validateSampleGeoJSON(req.Geometry);err!=nil{writeError(w,400,err.Error());return};var id string;err:=s.db.QueryRow(r.Context(),`INSERT INTO samples(sample_set_id,geometry,class_code,source,observation_start,observation_end) SELECT $1,ST_SetSRID(ST_GeomFromGeoJSON($2),4326),$3,$4,NULLIF($5,'')::timestamptz,NULLIF($6,'')::timestamptz FROM sample_sets WHERE id=$1 AND workspace_id=$7 RETURNING samples.id`,setID,string(req.Geometry),req.ClassCode,req.Source,req.ObservationStart,req.ObservationEnd,workspace(r)).Scan(&id);if err!=nil{if errors.Is(err,pgx.ErrNoRows){writeError(w,404,"conjunto de amostras não encontrado");return};writeError(w,500,err.Error());return};writeJSON(w,201,map[string]string{"id":id,"status":"draft"});return}
	if action=="train"&&r.Method==http.MethodPost{writeJSON(w,409,map[string]interface{}{"status":"blocked","message":"treinamento exige embeddings recentes, duas ou mais classes, splits por bloco/período e revisão das métricas; use workers/geo_ml/train_classifier.py e registre o artefato antes da aprovação"});return}
	writeError(w,405,"método não permitido")
}

func decode(w http.ResponseWriter,r *http.Request,dst interface{})bool{dec:=json.NewDecoder(io.LimitReader(r.Body,2<<20));if err:=dec.Decode(dst);err!=nil{writeError(w,400,"JSON inválido: "+err.Error());return false};return true}
func writeJSON(w http.ResponseWriter,status int,v interface{}){w.Header().Set("Content-Type","application/json");w.WriteHeader(status);_=json.NewEncoder(w).Encode(v)}
func writeError(w http.ResponseWriter,status int,message string){writeJSON(w,status,map[string]interface{}{"error":map[string]string{"message":message}})}
func validatePeriod(start,end string)error{a,err:=time.Parse(time.RFC3339,start);if err!=nil{return fmt.Errorf("start deve ser RFC3339")};b,err:=time.Parse(time.RFC3339,end);if err!=nil{return fmt.Errorf("end deve ser RFC3339")};if !a.Before(b){return fmt.Errorf("start deve ser anterior a end")};if b.After(time.Now().UTC().Add(5*time.Minute)){return fmt.Errorf("end não pode estar no futuro")};if b.Sub(a)>180*24*time.Hour{return fmt.Errorf("janela máxima é 180 dias")};return nil}
func timeValue(value string)time.Time{v,_:=time.Parse(time.RFC3339,value);return v}
func daysBetween(start,end string)int{return int(timeValue(end).Sub(timeValue(start))/24/time.Hour)}
func _unused(_ int){_ = strconv.IntSize}
