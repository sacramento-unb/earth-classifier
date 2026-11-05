'use client';

import dynamic from 'next/dynamic';
import { useEffect, useMemo, useState } from 'react';
import type { Area, GeoJSONGeometry, Run, Scene } from '../lib/contracts';
import { api } from '../lib/api';

const MapEditor = dynamic(() => import('../components/MapEditor'), { ssr: false });

type Project = { id: string; name: string; description: string };
type Availability = { sceneCount: number; scenes: Scene[]; note: string };
type ClassInfo = { code: string; name: string; color: string; technical: boolean };

const today = new Date().toISOString().slice(0, 10);
const initialPoints: [number, number][] = [];

export default function Home() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [project, setProject] = useState<Project | null>(null);
  const [areas, setAreas] = useState<Area[]>([]);
  const [area, setArea] = useState<Area | null>(null);
  const [points, setPoints] = useState(initialPoints);
  const [projectName, setProjectName] = useState('');
  const [areaName, setAreaName] = useState('Área de análise');
  const [start, setStart] = useState(() => shiftDate(today, 90));
  const [end, setEnd] = useState(today);
  const [availability, setAvailability] = useState<Availability | null>(null);
  const [run, setRun] = useState<Run | null>(null);
  const [models, setModels] = useState<Array<{ code: string; status: string }>>([]);
  const [classes, setClasses] = useState<ClassInfo[]>([]);
  const [sampleSetId, setSampleSetId] = useState('');
  const [sampleClass, setSampleClass] = useState('native_vegetation');
  const [sampleMessage, setSampleMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');

  useEffect(() => { void load(); }, []);
  async function load() {
    try {
      const [ps, ms, ts] = await Promise.all([api<Project[]>('/api/v1/projects'), api<Array<{ code: string; status: string }>>('/api/v1/models'), api<Array<{ classes: ClassInfo[] }>>('/api/v1/taxonomies')]);
      setProjects(ps); setModels(ms); setClasses(ts[0]?.classes ?? []);
      if (ps[0]) await selectProject(ps[0]);
    } catch (error) { setMessage((error as Error).message); }
  }
  async function selectProject(next: Project) { setProject(next); const loaded = await api<Area[]>(`/api/v1/areas?projectId=${next.id}`); setAreas(loaded); }
  async function createProject() {
    if (!projectName.trim()) return setMessage('Informe um nome para o projeto.');
    setBusy(true); try { const created = await api<Project>('/api/v1/projects', { method: 'POST', body: JSON.stringify({ name: projectName }) }); setProjects((old) => [created, ...old]); setProjectName(''); await selectProject(created); setMessage('Projeto criado.'); } catch (error) { setMessage((error as Error).message); } finally { setBusy(false); }
  }
  const geometry: GeoJSONGeometry | null = points.length >= 3 ? { type: 'Polygon', coordinates: [[...points.map(([lat, lng]) => [lng, lat]), [points[0][1], points[0][0]]]] } : null;
  async function saveArea() {
    if (!project || !geometry) return setMessage('Clique no mapa pelo menos três vezes para formar um polígono.');
    setBusy(true); try { const created = await api<Area>('/api/v1/areas', { method: 'POST', body: JSON.stringify({ projectId: project.id, name: areaName, geometry }) }); setAreas((old) => [created, ...old]); setArea(created); setMessage(`Área validada: ${created.areaHa.toFixed(2)} ha.`); } catch (error) { setMessage((error as Error).message); } finally { setBusy(false); }
  }
  async function checkAvailability() {
    if (!geometry) return setMessage('Salve uma área válida antes de consultar cenas.');
    setBusy(true); try { setAvailability(await api<Availability>('/api/v1/availability', { method: 'POST', body: JSON.stringify({ geometry, start: iso(start), end: iso(end, true) }) })); setMessage('Disponibilidade consultada no catálogo STAC real.'); } catch (error) { setMessage((error as Error).message); } finally { setBusy(false); }
  }
  async function createRun() {
    if (!project || !area) return setMessage('Salve e selecione uma área antes de processar.');
    setBusy(true); try { const queued = await api<{ id: string }>('/api/v1/runs', { method: 'POST', body: JSON.stringify({ projectId: project.id, areaId: area.id, start: iso(start), end: iso(end, true), sensors: ['sentinel2', 'sentinel1'] }) }); setRun(await api<Run>(`/api/v1/runs/${queued.id}`)); setMessage('Execução criada; esta página pode ser fechada e reaberta.'); } catch (error) { setMessage((error as Error).message); } finally { setBusy(false); }
  }
  async function createSampleSet() {
    if (!project) return setSampleMessage('Selecione um projeto.');
    try { const created = await api<{ id: string }>('/api/v1/sample-sets', { method: 'POST', body: JSON.stringify({ projectId: project.id, name: `Amostras · ${project.name}` }) }); setSampleSetId(created.id); setSampleMessage('Conjunto criado. Registre amostras revisadas antes de treinar.'); } catch (error) { setSampleMessage((error as Error).message); }
  }
  async function addSample() {
    if (!sampleSetId || !geometry) return setSampleMessage('Crie o conjunto e desenhe uma geometria de amostra.');
    try { await api(`/api/v1/sample-sets/${sampleSetId}/samples`, { method: 'POST', body: JSON.stringify({ geometry, classCode: sampleClass, source: 'revisão manual local' }) }); setSampleMessage('Amostra registrada como rótulo revisável.'); } catch (error) { setSampleMessage((error as Error).message); }
  }
  async function requestTraining() {
    if (!sampleSetId) return setSampleMessage('Crie um conjunto de amostras primeiro.');
    try { await api(`/api/v1/sample-sets/${sampleSetId}/train`, { method: 'POST' }); } catch (error) { setSampleMessage((error as Error).message); }
  }
  useEffect(() => { if (!run || ['succeeded', 'partial', 'insufficient_data', 'failed', 'cancelled'].includes(run.status)) return; const id = window.setInterval(() => { void api<Run>(`/api/v1/runs/${run.id}`).then(setRun).catch(() => undefined); }, 2500); return () => window.clearInterval(id); }, [run]);
  const approved = useMemo(() => models.some((model) => model.status === 'approved'), [models]);

  return <main>
    <header className="topbar"><div><span className="eyebrow">earth classifier</span><h1>Cobertura recente</h1><p>Observações de satélite dos últimos meses, com período e qualidade explícitos.</p></div><div className="status-chip">{approved ? 'modelo aprovado disponível' : 'sem modelo operacional aprovado'}</div></header>
    <div className="shell">
      <section className="notice"><strong>Regra científica:</strong> uma execução sem classificador aprovado pode consultar cenas e preparar dados, mas não publica mapa temático. “Não determinado” e “dados insuficientes” permanecem distintos.</section>
      <div className="grid">
        <section className="card"><div className="section-heading"><div><span className="eyebrow">01 · contexto</span><h2>Projeto e área</h2></div></div>
          <div className="row"><select value={project?.id ?? ''} onChange={(e) => { const selected = projects.find((item) => item.id === e.target.value); if (selected) void selectProject(selected); }}><option value="">Selecione um projeto</option>{projects.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select><input value={projectName} onChange={(e) => setProjectName(e.target.value)} placeholder="Novo projeto" /><button onClick={createProject} disabled={busy}>Criar</button></div>
          <div className="map-label"><label>Desenhe a área clicando no mapa</label><button className="link-button" onClick={() => setPoints([])}>Limpar vértices</button></div><MapEditor points={points} onChange={setPoints} />
          <div className="row"><input value={areaName} onChange={(e) => setAreaName(e.target.value)} /><button onClick={saveArea} disabled={busy || !project}>Salvar área</button></div>
          {areas.length > 0 && <label>Áreas cadastradas<select value={area?.id ?? ''} onChange={(e) => setArea(areas.find((item) => item.id === e.target.value) ?? null)}><option value="">Selecione</option>{areas.map((item) => <option key={item.id} value={item.id}>{item.name} · {item.areaHa.toFixed(1)} ha</option>)}</select></label>}
        </section>
        <section className="card"><div className="section-heading"><div><span className="eyebrow">02 · observações</span><h2>Janela recente</h2></div></div>
          <div className="periods">{[30, 60, 90, 180].map((days) => <button key={days} className={daysBetween(start, end) === days ? 'selected' : ''} onClick={() => setStart(shiftDate(end, days))}>{days} dias</button>)}</div>
          <div className="row"><label>Início<input type="date" value={start} max={end} onChange={(e) => setStart(e.target.value)} /></label><label>Fim<input type="date" value={end} max={today} onChange={(e) => setEnd(e.target.value)} /></label></div>
          <p className="hint">A janela é convertida em instantes UTC na execução. Datas efetivas podem ser menores conforme as cenas disponíveis; nenhuma observação posterior ao fim solicitado entra no processamento.</p>
          <button className="secondary wide" onClick={checkAvailability} disabled={busy}>Consultar disponibilidade real</button>
          {availability && <div className="availability"><strong>{availability.sceneCount} cenas candidatas</strong><span>S2 L2A e S1 RTC · máscara por pixel será avaliada depois</span>{availability.scenes.slice(0, 6).map((scene) => <div className="scene" key={`${scene.collection}-${scene.id}`}><span>{scene.collection === 'sentinel-2-l2a' ? 'S2' : 'S1'}</span><code>{scene.id}</code><time>{new Date(scene.observedAt).toLocaleDateString('pt-BR')}</time></div>)}</div>}
        </section>
      </div>
      <section className="card process"><div className="section-heading"><div><span className="eyebrow">03 · processamento</span><h2>Gerar análise</h2></div><button onClick={createRun} disabled={busy || !area}>Iniciar job assíncrono</button></div>{message && <p className="message">{message}</p>}{run && <RunStatus run={run} />}</section>
      <section className="card"><div className="section-heading"><div><span className="eyebrow">04 · amostras</span><h2>Treinar um classificador compatível</h2></div><button onClick={createSampleSet} disabled={!project}>Criar conjunto</button></div><p className="hint">Amostras precisam de classe, fonte, período e revisão. O treino não transforma automaticamente uma correção em verdade.</p><div className="row"><select value={sampleClass} onChange={(e) => setSampleClass(e.target.value)}>{classes.filter((item) => !item.technical).map((item) => <option key={item.code} value={item.code}>{item.name}</option>)}</select><button className="secondary" onClick={addSample} disabled={!sampleSetId || !geometry}>Registrar geometria atual</button><button className="secondary" onClick={requestTraining} disabled={!sampleSetId}>Tentar treinar</button></div>{sampleMessage && <p className="message">{sampleMessage}</p>}</section>
      <section className="card"><div className="section-heading"><div><span className="eyebrow">05 · legenda e governança</span><h2>Classes disponíveis</h2></div></div><div className="legend">{classes.map((item) => <div key={item.code}><i style={{ background: item.color }} />{item.name}{item.technical && <small>técnica</small>}</div>)}</div><p className="hint">A legenda operacional é versionada e mantém relação auxiliar com o MapBiomas; não substitui validação de campo nem transforma um rótulo histórico em verdade recente.</p></section>
      <section className="card"><div className="section-heading"><div><span className="eyebrow">histórico</span><h2>Áreas e execuções</h2></div></div><p className="hint">{areas.length} área(s) neste projeto · {models.length ? `${models.length} modelo(s) registrado(s)` : 'nenhum modelo registrado'}</p></section>
    </div>
  </main>;
}

function RunStatus({ run }: { run: Run }) { return <div className="run-status"><div className="run-title"><strong>{run.status === 'insufficient_data' ? 'Dados insuficientes para publicar um mapa' : `Execução ${run.status}`}</strong><code>{run.id}</code></div><div className="steps">{(run.steps ?? []).map((step) => <div key={step.name} className={step.status}><span>{step.status === 'succeeded' ? '✓' : step.status === 'skipped' ? '–' : '·'}</span>{step.name}</div>)}</div><p className="hint">{run.status === 'insufficient_data' ? 'As cenas e metadados foram consultados. O próximo passo é criar amostras compatíveis, treinar/avaliar e promover explicitamente um classificador.' : 'O status é persistido no servidor e a tela consulta novamente sem manter requisição longa.'}</p></div>; }
function iso(value: string, end = false) { if (end && value === today) return new Date().toISOString(); return `${value}T${end ? '23:59:59.999' : '00:00:00'}Z`; }
function shiftDate(value: string, days: number) { const d = new Date(`${value}T00:00:00Z`); d.setUTCDate(d.getUTCDate() - days); return d.toISOString().slice(0, 10); }
function daysBetween(start: string, end: string) { return Math.round((Date.parse(`${end}T00:00:00Z`) - Date.parse(`${start}T00:00:00Z`)) / 86400000); }
