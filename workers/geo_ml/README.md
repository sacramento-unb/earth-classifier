# worker científico

O worker científico é deliberadamente separado da API Go. `probe_recent_olmoearth.py` consulta o Planetary Computer, lê uma área pequena em COG, aplica máscaras SCL, registra o contrato de bandas e, com `--run-model`, carrega o checkpoint v1.3 por revisão fixa e escreve um GeoTIFF de embeddings.

No Compose, ele fica no profile opcional `science`:

```bash
docker compose --profile science run --rm science \
  workers/geo_ml/probe_recent_olmoearth.py --out artifacts/probe-recent
```

## prova técnica

Com Docker e sem GPU, a preparação de dados reais pode ser verificada sem instalar nada no host:

```bash
docker run --rm -v "$PWD":/work -w /work python:3.12-slim bash -lc \
  "apt-get update -qq && apt-get install -y -qq libexpat1 && \
   pip install -r workers/geo_ml/requirements-probe.txt && \
   python workers/geo_ml/probe_recent_olmoearth.py --out artifacts/probe-recent"
```

Para baixar aproximadamente 3.5 GB do checkpoint e medir inferência CPU, acrescente `--run-model`. O script registra `manifest.json`, shapes, dimensões 128/64, datas, sensores, CRS, resolução, checksum e tempo; não publica classificação. GPU é detectada, mas não está disponível no ambiente de desenvolvimento atual.

O código oficial atual usa `load_model_from_path` porque o `ModelID` publicado ainda não contém uma constante v1.3. O checkpoint é carregado por `config.json` + `weights.pth` do repositório HF com revisão fixa. A saída operacional usa `student_location_tokens` (128 dimensões, prefixo 64), não o `tokens_and_masks` de 768 dimensões do tutorial rslearn antigo. O eixo temporal é comum às modalidades; buckets sem S2 recebem máscara `MISSING`, nunca zero interpretado como observação válida.

## treinamento e aplicação

`train_classifier.py` exige embeddings recentes rotulados em NPZ (`X`, `y`, `groups`, `periods`) e separa fazendas/blocos com `GroupShuffleSplit`; não aceita dataset sem pelo menos duas classes. Ele cria um modelo `draft`. Para aplicar, `apply_classifier.py` exige `model.json` com `status=approved`, checkpoint e dimensão exatamente compatíveis. Escores abaixo do limiar viram `254` (não determinado), nunca uma classe temática.

Rótulos históricos podem ser usados como referência auxiliar, mas não bastam como teste independente de uma execução recente. A promoção para aprovado é deliberadamente uma ação humana registrada na API/banco.
