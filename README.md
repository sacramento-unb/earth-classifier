# Earth classifier

Sistema WebGIS para consultar observações recentes, preparar embeddings OlmoEarth e publicar classificação somente quando houver um classificador supervisionado aprovado e compatível. A implementação usa Next.js/TypeScript, API/orquestrador Go, PostgreSQL/PostGIS e Python isolado para sensoriamento remoto e ML.

## executar localmente

Pré-requisitos: Docker com Compose. Go não precisa estar instalado no host; os containers compilam a API e o orquestrador. Node só é necessário para executar a web fora do Compose.

```bash
cp .env.example .env
docker compose up --build
```

Abra [http://localhost:3000](http://localhost:3000). A API fica em [http://localhost:8080/healthz](http://localhost:8080/healthz). O banco PostGIS, MinIO e o serviço TiTiler opcional são definidos no Compose.

Fluxo vertical disponível: criar projeto → desenhar polígono → salvar/validar área → consultar disponibilidade real no Planetary Computer → iniciar job → acompanhar polling. Sem modelo aprovado, o worker persiste cenas e termina como `insufficient_data`; não há raster fictício.

## prova técnica OlmoEarth

```bash
docker run --rm -v "$PWD":/work -w /work python:3.12-slim bash -lc \
  "apt-get update -qq && apt-get install -y -qq libexpat1 && \
   pip install -r workers/geo_ml/requirements-probe.txt && \
   python workers/geo_ml/probe_recent_olmoearth.py --out artifacts/probe-recent"
```

Use `--run-model` para baixar o checkpoint v1.3 de aproximadamente 3.5 GB e executar inferência. A prova real reproduzível está em [proof/olmoearth-v1_3](proof/olmoearth-v1_3), incluindo manifesto, entradas e GeoTIFF de 128 bandas. O smoke test atual não exige credencial do Planetary Computer, mas pode depender de disponibilidade do catálogo e de transferência de COGs. GPU é opcional; este ambiente foi detectado sem driver NVIDIA.

## amostras e modelo

Os scripts Python exigem labels recentes e grupos espaciais. Gere um NPZ com `X`, `y`, `groups` e `periods` e treine:

```bash
python workers/geo_ml/train_classifier.py \
  --dataset samples/recent_embeddings.npz \
  --out artifacts/model-v1 \
  --embedding-dim 128 \
  --checkpoint-revision 9e9e918a369701c105dde7ef56789b0ff1d47f50 \
  --taxonomy operational_broad_v1 \
  --sensors sentinel2-l2a sentinel1-rtc
```

O comando cria `draft`. A promoção para `approved` é intencional e deve ser feita após revisão das métricas, splits espaciais/temporais, calibração e escopo de validade. `apply_classifier.py` recusa modelos não aprovados ou incompatíveis.

O worker científico também pode ser executado em container, sem instalar Python no host:

```bash
docker compose --profile science build science
docker compose --profile science run --rm science \
  workers/geo_ml/probe_recent_olmoearth.py --out artifacts/probe-recent --run-model
```

O profile `science` é opcional porque instala PyTorch e o adaptador OlmoEarth; o cache do checkpoint fica no volume `huggingface-cache`.

## documentação

- [docs/architecture.md](docs/architecture.md): decisões e limites do MVP.
- [docs/technical-proof.md](docs/technical-proof.md): fontes, versões e prova real registrada.
- [docs/sampling.md](docs/sampling.md): guia para amostragem, treinamento e aprovação.
- [packages/contracts/openapi.yaml](packages/contracts/openapi.yaml): contrato REST.
- [workers/geo_ml/README.md](workers/geo_ml/README.md): detalhes do adaptador v1.3 e do worker científico.

## limitações explícitas

O ambiente inicial não possui amostras rotuladas, modelo downstream aprovado, Go instalado localmente ou GPU disponível. Por isso o produto não emite classificação operacional ainda. A autenticação do Compose é somente isolamento de desenvolvimento por `X-Workspace-ID`; uma implantação real precisa conectar um provedor de identidade e validar sessão/roles no middleware antes de exposição externa.
