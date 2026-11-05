# prova técnica registrada — 23/10/2026

## fontes consultadas

- Ai2, [anúncio OlmoEarth v1.3](https://allenai.org/blog/olmoearth-v1-3), publicado em 22/10/2026. Descreve embeddings globais anuais 10 m e 64/128 dimensões.
- Ai2, [relatório técnico v1.3](https://allenai.org/papers/olmoearth-v1-3), consultado em 23/10/2026. O modelo é multimodal/multitemporal; a projeção compacta é distinta dos tokens de 768 dimensões.
- AllenAI, [olmoearth_pretrain](https://github.com/allenai/olmoearth_pretrain), commit usado na inspeção: `7353d1b5105a3ad662809a700c98065fc711f4ab`.
- Hugging Face, [OlmoEarth-v1_3-Base](https://huggingface.co/allenai/OlmoEarth-v1_3-Base), revisão usada: `9e9e918a369701c105dde7ef56789b0ff1d47f50`; checkpoint com `config.json` e `weights.pth`.
- AllenAI, [tutorial rslearn de embeddings](https://github.com/allenai/rslearn/blob/master/docs/examples/OlmoEarthEmbeddings.md), consultado em 23/10/2026. O exemplo permanece em `OLMOEARTH_V1_BASE`, usa 768 bandas e por isso não foi copiado como receita v1.3.
- MapBiomas Brasil, [códigos de legenda](https://brasil.mapbiomas.org/downloads/codigos-de-legenda/), consultado em 23/10/2026. A legenda operacional deste repositório registra a relação como auxiliar, não como substituição.

## experimento real executado

`probe_recent_olmoearth.py` consultou o STAC do Microsoft Planetary Computer para uma área pequena em São Paulo (`-46.650,-23.580,-46.640,-23.570`) e a janela absoluta `[2026-07-25T00:00:00Z, 2026-10-23T23:59:59Z]`. A leitura COG foi feita em uma grade 64×64, EPSG:32723, 10 m, com Sentinel-2 L2A e Sentinel-1 RTC.

O manifesto persistido em [`proof/olmoearth-v1_3/manifest.json`](../proof/olmoearth-v1_3/manifest.json) registrou duas cenas S2 (17/09 e 27/09), três cenas S1 RTC (20/08, 19/09 e 19/10), bandas S2 na ordem oficial `B02,B03,B04,B08,B05,B06,B07,B8A,B11,B12,B01,B09` e S1 `vv,vh`. A preparação levou aproximadamente 41 s no container CPU. A máscara S2 observou somente classes SCL 8/9 na janela efetiva; a fração óptica válida ficou em 0,0, enquanto S1 ficou em 1,0. O manifesto preserva a causa/diagnóstico e o bucket ausente como máscara, sem fabricar pixels válidos.

O adaptador v1.3 usa `load_model_from_path` sobre o snapshot HF, chama o encoder oficial com `patch_size=4` e lê `student_location_tokens`: 128 dimensões, com prefixo 64. Como o encoder exige um eixo temporal comum entre modalidades, S2/S1 são alinhados em três buckets de 30 dias e a ausência óptica fica mascarada, não preenchida como classe. A inferência real foi executada em CPU com `torch 2.9.0+cpu`: saída `128×16×16`, 40 m por célula para entrada 10 m/patch 4, prefixo 64 disponível, tempo total aproximado de 45 s após o checkpoint estar em cache e checksum SHA-256 `5ae47fa765a7db307a19e2279443d672060d2e6defa584d16028802ea6c77935`. O GeoTIFF, entradas e manifesto persistidos ficam em `proof/olmoearth-v1_3/`.

## pendência da prova

A prova do encoder foi concluída para este tile pequeno e receita observada. Isso não aprova um classificador downstream: ainda faltam amostras rotuladas, treinamento, avaliação espacial/temporal, calibração e aprovação para publicar mapa. A medição é CPU e não deve ser extrapolada para produção; a prova deve ser repetida em hardware dimensionado e em regiões/janelas adicionais.
