package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type worker struct { db *pgxpool.Pool; stacURL string }
type stacResponse struct { Features []struct { ID string `json:"id"`; Collection string `json:"collection"`; Properties map[string]interface{} `json:"properties"`; Assets map[string]struct { Href string `json:"href"` } `json:"assets"` } `json:"features"` }

func main() {
	db, err := pgxpool.New(context.Background(), env("DATABASE_URL", "postgres://earth:earth@localhost:5432/earth_classifier?sslmode=disable")); if err != nil { panic(err) }; defer db.Close()
	if err := db.Ping(context.Background()); err != nil { panic(err) }
	w := &worker{db: db, stacURL: env("STAC_API_URL", "https://planetarycomputer.microsoft.com/api/stac/v1")}
	slog.Info("orchestrator started")
	for { if err := w.tick(context.Background()); err != nil { slog.Error("worker tick failed", "error", err) }; time.Sleep(2 * time.Second) }
}

func env(key, fallback string) string { if v := os.Getenv(key); v != "" { return v }; return fallback }

func (w *worker) tick(ctx context.Context) error {
	var jobID, runID string
	err := w.db.QueryRow(ctx, `WITH claim AS (SELECT id FROM jobs WHERE status='queued' OR (status='running' AND lease_until < now()) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE jobs j SET status='running', attempts=j.attempts+1, lease_until=now()+interval '10 minutes', heartbeat_at=now() FROM claim WHERE j.id=claim.id RETURNING j.id,j.run_id`).Scan(&jobID, &runID)
	if err != nil { if err == pgx.ErrNoRows { return nil }; return err }
	return w.process(ctx, jobID, runID)
}

func (w *worker) process(ctx context.Context, jobID, runID string) error {
	if err := w.step(ctx, jobID, "validate", "Geometria e intervalo já foram validados pela API"); err != nil { return w.fail(ctx, jobID, runID, err) }
	var geometry string; var start, end time.Time
	if err := w.db.QueryRow(ctx, `SELECT ST_AsGeoJSON(a.geometry), cr.requested_start, cr.requested_end FROM classification_runs cr JOIN areas a ON a.id=cr.area_id WHERE cr.id=$1`, runID).Scan(&geometry, &start, &end); err != nil { return w.fail(ctx, jobID, runID, err) }
	if err := w.stepRunning(ctx, jobID, "availability"); err != nil { return w.fail(ctx, jobID, runID, err) }
	scenes, err := w.search(ctx, geometry, start, end); if err != nil { return w.fail(ctx, jobID, runID, err) }
	if err := w.persistScenes(ctx, jobID, runID, scenes); err != nil { return w.fail(ctx, jobID, runID, err) }
	if len(scenes) == 0 { return w.insufficient(ctx, jobID, runID, "Nenhuma cena elegível no intervalo solicitado") }
	if err := w.stepDone(ctx, jobID, "availability", map[string]interface{}{"sceneCount": len(scenes)}); err != nil { return w.fail(ctx, jobID, runID, err) }
	for _, name := range []string{"materialize", "embeddings", "classify", "quality", "publish"} { if err := w.stepSkipped(ctx, jobID, name, "Bloqueado até existir modelo aprovado e worker científico habilitado"); err != nil { return w.fail(ctx, jobID, runID, err) } }
	return w.insufficient(ctx, jobID, runID, "Dados consultados; não há classificador aprovado compatível. Crie amostras, treine e aprove uma versão antes de publicar um mapa.")
}

func (w *worker) search(ctx context.Context, geometry string, start, end time.Time) ([]map[string]interface{}, error) {
	result := []map[string]interface{}{}
	for _, collection := range []string{"sentinel-2-l2a", "sentinel-1-rtc"} {
		query := map[string]interface{}{"collections": []string{collection}, "datetime": start.Format(time.RFC3339)+"/"+end.Format(time.RFC3339), "intersects": json.RawMessage(geometry), "limit": 100}
		if collection == "sentinel-2-l2a" { query["query"] = map[string]interface{}{"eo:cloud_cover": map[string]interface{}{"lt": 80}} }
		body,_:=json.Marshal(query); req,_:=http.NewRequestWithContext(ctx,http.MethodPost,w.stacURL+"/search",strings.NewReader(string(body)));req.Header.Set("Content-Type","application/json")
		resp,err:=http.DefaultClient.Do(req);if err!=nil{return nil,err};data,readErr:=io.ReadAll(io.LimitReader(resp.Body,20<<20));resp.Body.Close();if readErr!=nil{return nil,readErr};if resp.StatusCode>=300{return nil,fmt.Errorf("STAC HTTP %d",resp.StatusCode)}
		var parsed stacResponse;if err:=json.Unmarshal(data,&parsed);err!=nil{return nil,err};for _,f:=range parsed.Features{assets:=map[string]string{};for _,key:=range []string{"B02","B03","B04","B08","B05","B06","B07","B8A","B11","B12","B01","B09","SCL","vv","vh"}{if a,ok:=f.Assets[key];ok{assets[key]=a.Href}};result=append(result,map[string]interface{}{"id":f.ID,"collection":collection,"observedAt":f.Properties["datetime"],"cloudCover":f.Properties["eo:cloud_cover"],"assets":assets})}
	}
	return result,nil
}

func (w *worker) persistScenes(ctx context.Context, jobID, runID string, scenes []map[string]interface{}) error {
	for _, scene := range scenes { raw,_:=json.Marshal(scene); id:=scene["id"].(string); collection:=scene["collection"].(string); observed,_:=time.Parse(time.RFC3339,fmt.Sprint(scene["observedAt"]));var sceneID string; if err:=w.db.QueryRow(ctx,`INSERT INTO scene_assets(provider,collection,scene_id,observed_at,cloud_cover,metadata) VALUES('planetary-computer',$1,$2,$3,$4,$5) ON CONFLICT(provider,collection,scene_id) DO UPDATE SET metadata=EXCLUDED.metadata RETURNING id`,collection,id,observed,scene["cloudCover"],raw).Scan(&sceneID);err!=nil{return err}; role:="radar";if collection=="sentinel-2-l2a"{role="optical"};if _,err:=w.db.Exec(ctx,`INSERT INTO run_scene_assets(run_id,scene_asset_id,role) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`,runID,sceneID,role);err!=nil{return err} }
	_,err:=w.db.Exec(ctx,`UPDATE classification_runs SET effective_start=(SELECT min(sa.observed_at) FROM run_scene_assets rsa JOIN scene_assets sa ON sa.id=rsa.scene_asset_id WHERE rsa.run_id=$1), effective_end=(SELECT max(sa.observed_at) FROM run_scene_assets rsa JOIN scene_assets sa ON sa.id=rsa.scene_asset_id WHERE rsa.run_id=$1), manifest=jsonb_set(manifest,'{sceneCount}',to_jsonb($2::int)), updated_at=now() WHERE id=$1`,runID,len(scenes));return err
}

func (w *worker) step(ctx context.Context, jobID, name, message string) error { if err:=w.stepRunning(ctx,jobID,name);err!=nil{return err};return w.stepDone(ctx,jobID,name,map[string]string{"message":message}) }
func (w *worker) stepRunning(ctx context.Context,jobID,name string)error{_,err:=w.db.Exec(ctx,`UPDATE job_steps SET status='running',attempts=attempts+1,started_at=now() WHERE job_id=$1 AND name=$2`,jobID,name);return err}
func (w *worker) stepDone(ctx context.Context,jobID,name,payload interface{})error{raw,_:=json.Marshal(payload);_,err:=w.db.Exec(ctx,`UPDATE job_steps SET status='succeeded',finished_at=now(),output_refs=$3 WHERE job_id=$1 AND name=$2`,jobID,name,raw);if err==nil{_,err=w.db.Exec(ctx,`INSERT INTO job_events(job_id,event_type,message,payload) VALUES($1,'progress',$2,$3)`,jobID,name,raw)};return err}
func (w *worker) stepSkipped(ctx context.Context,jobID,name,message string)error{_,err:=w.db.Exec(ctx,`UPDATE job_steps SET status='skipped',finished_at=now(),error=jsonb_build_object('message',$3::text) WHERE job_id=$1 AND name=$2`,jobID,name,message);return err}
func (w *worker) insufficient(ctx context.Context,jobID,runID,message string)error{_,err:=w.db.Exec(ctx,`UPDATE jobs SET status='insufficient_data',lease_until=NULL WHERE id=$1`,jobID);if err!=nil{return err};_,err=w.db.Exec(ctx,`UPDATE classification_runs SET status='insufficient_data',manifest=jsonb_set(manifest,'{scientificStatus}',to_jsonb($2::text)),updated_at=now() WHERE id=$1`,runID,message);if err==nil{_,err=w.db.Exec(ctx,`INSERT INTO job_events(job_id,event_type,message) VALUES($1,'insufficient_data',$2)`,jobID,message)};return err}
func (w *worker) fail(ctx context.Context,jobID,runID string,reason error)error{msg:=reason.Error();_,_=w.db.Exec(ctx,`UPDATE jobs SET status='failed',last_error=jsonb_build_object('message',$2),lease_until=NULL WHERE id=$1`,jobID,msg);_,_=w.db.Exec(ctx,`UPDATE classification_runs SET status='failed',updated_at=now() WHERE id=$1`,runID);_,_=w.db.Exec(ctx,`INSERT INTO job_events(job_id,event_type,message) VALUES($1,'failed',$2)`,jobID,msg);return reason}
