# Prompt de implementação — classificação recente de uso e cobertura da terra

## 1. Missão e forma de execução

Atue como engenheiro de software sênior com experiência em WebGIS, sensoriamento remoto e aprendizado de máquina. Construa um sistema funcional, operado integralmente pelo navegador, para selecionar um polígono, obter observações de satélite dos últimos meses e produzir uma classificação de cobertura da terra com referência temporal explícita.

Implemente o produto, não entregue apenas planejamento ou telas estáticas. Primeiro inspecione o repositório e suas instruções, preserve trabalho existente e proponha uma arquitetura curta; em seguida execute em incrementos verificáveis. Registre decisões técnicas e limitações. Resolva escolhas rotineiras autonomamente; solicite apenas credenciais ou informações realmente indispensáveis. Não simule integração científica como se fosse inferência real.

Prioridades do usuário:

- Interface web em Next.js e TypeScript.
- Backend preferencialmente em Go e Node.js.
- Banco principal PostgreSQL com PostGIS.
- Classificação sempre baseada em observações recentes, com janela padrão inicial de 90 dias.
- Classes amplas de vegetação nativa, pastagem, agricultura, solo exposto, água e área construída, com detalhamento inspirado na legenda MapBiomas quando tecnicamente justificável.
- Operação simples: selecionar área, escolher período, processar, inspecionar, corrigir e exportar.

O prazo de 90 dias é uma hipótese inicial de produto, não uma janela validada cientificamente. Ofereça 30, 60, 90 e 180 dias e datas personalizadas, com indicação das configurações efetivamente validadas.

## 2. Premissas científicas obrigatórias

1. Utilize pesos pré-treinados do OlmoEarth, preferencialmente v1.3, para gerar embeddings das observações da janela solicitada; aplique um classificador supervisionado próprio. Não pré-treine um foundation model para cada polígono.
2. Embeddings anuais históricos não representam automaticamente o período atual. Não classifique embeddings antigos e apresente o resultado sobre uma imagem recente como se fosse uma atualização.
3. Embeddings históricos podem ser contexto opcional em um experimento separado, sempre com variáveis atuais e treinamento correspondente. Não são requisito do MVP.
4. A existência de suporte a períodos curtos não comprova acurácia suficiente nessa aplicação. Valide janelas, sensores e regiões antes de promover um classificador para uso operacional.
5. Diferencie cobertura observada no período e uso provável da terra. Uma lavoura colhida pode apresentar solo exposto; pastagem seca pode se confundir com vegetação campestre. Não infira manejo, origem nativa ou espécie vegetal sem evidências e validação.
6. Uma janela de vários meses não equivale a uma fotografia do último dia. Exiba início e fim solicitados, datas efetivas, disponibilidade e idade das observações. Não use a expressão “tempo real”.
7. Falta de dados e baixa confiança são situações diferentes. Preserve estados distintos para sem observação suficiente, não determinado pelo classificador e fora do polígono.
8. Sem um classificador aprovado e compatível, permita preparar dados e gerar embeddings, mas bloqueie a emissão de um mapa operacional. Ofereça o fluxo de amostragem, treinamento e validação. Nunca invente uma classificação universal pronta.

## 3. Prova técnica obrigatória antes da aplicação completa

Consulte fontes oficiais atuais e registre URLs, datas, versões e commits usados:

- https://allenai.org/blog/olmoearth-v1-3
- https://allenai.org/papers/olmoearth-v1-3
- https://github.com/allenai/olmoearth_pretrain
- https://huggingface.co/allenai/OlmoEarth-v1_3-Base
- https://github.com/allenai/rslearn/blob/master/docs/examples/OlmoEarthEmbeddings.md
- https://brasil.mapbiomas.org/codigos-de-legenda/

Atenção: na consulta de referência de 23/10/2026, o tutorial rslearn de embeddings ainda continha um exemplo com OLMOEARTH_V1_BASE. Não copie esse exemplo assumindo que gera os embeddings compactos da v1.3. Verifique a API real, o checkpoint e a cabeça de embeddings da versão escolhida. Não invente identificadores, assinaturas, formatos ou endpoints.

Execute um experimento pequeno com dados reais:

1. Uma área pequena no Brasil e uma janela recente de 90 dias.
2. Obtenção e preparação de Sentinel-2; integração Sentinel-1 conforme o suporte real.
3. Carregamento do checkpoint escolhido e inferência reproduzível.
4. Verificação de bandas, ordem, unidades, normalização, datas, máscaras, sensores ausentes, dimensões e resolução espacial de saída.
5. Confirmação do comportamento da agregação temporal, projeção compacta e opções de 64/128 dimensões na v1.3.
6. Exportação georreferenciada e abertura independente para verificar alinhamento espacial.
7. Medição de tempo, RAM, VRAM, volume transferido e armazenamento.

Use GPU quando disponível, com tiles e lotes configuráveis. Não prometa funcionamento aceitável em uma GPU específica antes de medir. Um modo CPU pode existir se suportado, com limites e tempo medidos. Reduza tiles/lotes em caso de memória insuficiente, sem alterar silenciosamente a especificação do produto.

Se houver incompatibilidade entre o tutorial e a v1.3, implemente um adaptador com base no código oficial real e documente. Se o acesso ou os dados impedirem a prova, entregue o que for executável e um diagnóstico preciso; não declare a integração concluída.

## 4. Arquitetura preferencial

Adote um monorepo simples, com responsabilidades claras:

| Componente | Tecnologia preferencial | Responsabilidade |
| --- | --- | --- |
| Web | Next.js, TypeScript, Node.js | Interface, autenticação web e visualização cartográfica |
| API | Go | Regras de negócio, autorização, projetos, áreas, execuções, amostras e exportações |
| Orquestrador | Worker Go | Fila persistente, etapas, cancelamentos, retries e retomada |
| Worker científico | Python, PyTorch, bibliotecas geoespaciais | Pré-processamento especializado, OlmoEarth, treinamento e inferência |
| Dados | PostgreSQL + PostGIS | Geometrias, metadados, jobs, auditoria e registro de modelos |
| Arquivos | Armazenamento compatível com S3 | Cenas, recortes, embeddings, modelos e resultados |
| Tiles | Serviço compatível com COG, por exemplo TiTiler | Renderização sob demanda dos rasters |

Python é uma exceção deliberada à preferência por Go/Node: isole-o no processamento científico para reutilizar o ecossistema oficial. A API de negócio permanece em Go e o frontend usa Node. Não crie uma segunda API de negócio em Node sem necessidade concreta.

Prefira inicialmente uma fila de jobs em PostgreSQL, com bloqueio por linha, leases e recuperação após falha. Não adicione Redis, Kafka ou Kubernetes sem uma necessidade medida. Use polling incremental ou SSE para progresso; se usar SSE, valide reconexão, autorização e suporte do proxy.

O worker recebe IDs e referências de artefatos; nunca trafegue tensores volumosos no JSON da API. Não exponha serviços científicos, banco ou armazenamento diretamente ao público.

Estrutura sugerida: apps/web, services/api, services/orchestrator, workers/geo-ml, packages/contracts, infra, migrations e docs. Ajuste à organização existente.

## 5. Experiência do usuário

Interface em português, responsiva, com foco no uso em desktop:

- Lista de projetos, áreas e execuções anteriores.
- Mapa com desenho e edição de Polygon/MultiPolygon, upload GeoJSON e seleção de área cadastrada. GeoPackage pode ser uma segunda entrega.
- Resumo da área em hectares e validação da geometria antes de processar.
- Seleção de janela temporal e classes desejadas dentro dos modelos disponíveis.
- Prévia de disponibilidade: cenas candidatas, cobertura efetiva, datas e estimativa de recursos baseada em medições.
- Botão de processamento que cria job assíncrono e permite sair da página e retornar depois.
- Progresso por etapa, avisos de qualidade, cancelamento e mensagens de erro compreensíveis.
- Resultado com RGB recente, classificação, qualidade de observação, probabilidade/confiança e comparação visual por transparência ou swipe.
- Legenda, áreas por classe em hectares e percentuais, área sem dados e área não determinada.
- Inspetor de pixel com classe, escores, origem dos dados e resumo temporal disponível.
- Editor de amostras, importação de rótulos e revisão de classificação.
- Exportações e histórico de versões.

O período considerado e a data da observação válida mais recente devem aparecer no resultado. RGB de referência e classificação podem ser produtos diferentes: informe isso quando não representarem as mesmas datas.

Não coloque termos como checkpoint, tensor ou atenção na jornada comum. Detalhes técnicos ficam em uma seção avançada.

## 6. Aquisição e preparação dos dados

Use catálogos STAC ou conectores oficiais verificados, com adaptador de provedor. Avalie rslearn para aproveitar o caminho oficial de obtenção e preparação de entradas. Caso use outro pipeline, comprove equivalência às entradas esperadas.

- Priorize Sentinel-2 L2A; avalie Sentinel-1 como complemento. Landsat é opcional, sem bloquear o MVP.
- Verifique disponibilidade, autenticação, custos e termos de cada provedor; implemente pelo menos um caminho real, em vez de supor que todos os sensores estão no mesmo catálogo.
- Resolva “últimos N dias” em datas absolutas ao criar a execução; armazene instantes em UTC e apresente datas conforme o contexto do usuário. Defina o intervalo e tratamento inclusivo/exclusivo.
- Não obtenha observações posteriores à data final solicitada. Não amplie a janela silenciosamente.
- Use interseção espacial, contexto ao redor da área e leitura por janela/COG quando viável; não baixe tiles completos desnecessariamente.
- Harmonize CRS, grade, extensão, bandas e unidades de acordo com o modelo. Não trate uma composição RGB como substituta da entrada multiespectral.
- Aplique máscaras de nuvens, sombras e pixels inválidos. Cobertura de nuvem da cena serve como filtro inicial, não como avaliação suficiente do polígono.
- Sentinel-1 exige produto e pré-processamento compatíveis; não misture escala linear, dB e produtos diferentes sem transformação validada.
- Use mosaicos temporais e cadência compatíveis com o modelo oficial. Documente e valide qualquer mudança nessa receita.
- Trate modalidades ausentes com o mecanismo oficial. Não transforme ausência em zero válido; registrar redução de sensores e compatibilidade com o classificador.
- Reamostre categorias por vizinho mais próximo e variáveis contínuas pelo método apropriado. Interpolar para uma grade de 10 m não cria detalhe nativo de 10 m em todos os sensores.
- Processe em tiles com contexto e remoção de halos; verifique costuras e georreferenciamento.
- Para áreas em múltiplas zonas UTM, defina particionamento ou CRS apropriado; calcule hectares em projeção adequada ou por método geodésico, nunca em graus nem presumindo equivalência de área em Web Mercator.

Gere métricas de qualidade, como número de observações ópticas válidas, datas efetivas, idade da observação mais recente e fração válida. Defina formalmente se cada métrica representa pixels de entrada, mosaicos ou cenas; não atribua uma data única a um embedding agregado.

Quando dados forem insuficientes, ofereça explicitamente outra janela ou sensores suportados. Se não houver informação suficiente, encerre com status de dados insuficientes ou produza resultado parcial sinalizado, conforme política configurada.

## 7. Classes e relação com o MapBiomas

Mantenha taxonomia versionada, com hierarquia, definição, cores e regras de compatibilidade por bioma. A lista abaixo é uma legenda operacional proposta, não uma reprodução literal da legenda oficial:

| Classe ampla | Detalhamento condicionado à validação |
| --- | --- |
| Vegetação nativa | Formações florestais, savânicas e campestres |
| Pastagem | Subclasses somente se houver evidência suficiente |
| Agricultura | Culturas ou grupos quando o período e as amostras permitirem |
| Silvicultura | Separação de vegetação nativa e outras culturas |
| Solo exposto | Estado recente, distinto de uso permanente |
| Água | Classes adicionais se validadas |
| Área construída | Detalhamento conforme escala e dados |
| Outras coberturas naturais não vegetadas | Areia, rocha e outras quando sustentadas por amostras |

Adicione estados técnicos próprios para não determinado e dados insuficientes, além do NoData fora da área. Não os confunda com classes temáticas oficiais.

Importe e versione a legenda oficial escolhida, indicando produto, coleção, resolução, URL e data. Não invente códigos MapBiomas nem imponha um código oficial a uma categoria apenas aproximada. O mapeamento pode ser exato, mais amplo, parcial ou inexistente; registre essas relações.

Não force todas as subclasses: só habilite classes com suporte no classificador escolhido. Fallback para classe parental exige modelo e regra de agregação validados; caso contrário, use não determinado. Distinguir pasto de formação campestre nativa é um caso obrigatório na validação.

MapBiomas pode fornecer rótulos históricos fracos ou referência auxiliar. Não substitua a inferência recente por um recorte MapBiomas, não trate seus mapas como verdade absoluta e não transfira rótulos antigos para áreas que mudaram sem revisão. Reamostrar um rótulo de 30 m para 10 m não produz amostras independentes nem verdade de campo de 10 m.

## 8. Amostragem, treinamento e aprovação de modelos

Implemente este ciclo antes da classificação operacional:

1. Criar conjunto de amostras por pontos/polígonos e importar arquivos com classe, data/período, fonte e revisão.
2. Amostrar classes, biomas, regiões, estações e situações difíceis, com controle de desbalanceamento.
3. Gerar embeddings para janelas equivalentes às usadas na operação; registrar sensores e política temporal.
4. Separar treino, validação e teste por fazenda/polígono ou blocos espaciais e por tempo, conforme objetivo. Pixels do mesmo talhão e janelas sobrepostas não podem vazar entre partições independentes.
5. Treinar uma referência simples com bandas, índices e estatísticas temporais + Random Forest, e comparar com embeddings recentes + Random Forest ou regressão logística.
6. Avaliar matriz de confusão, precision, recall, F1 e IoU por classe, macro-F1, cobertura de predição e erros por região/estação. Não escolher modelo só pela acurácia global.
7. Calibrar probabilidades quando aplicável em conjunto separado; se não calibradas, apresentar como escores. Confiança do classificador não é acurácia comprovada nem qualidade de observação.
8. Medir desempenho com abstenção e definir limiares por classe quando justificado. Guardar métricas antes e depois de pós-processamento.
9. Registrar limitações de domínio e critérios de aprovação configuráveis. Não inventar metas de acurácia atingidas.
10. Promover explicitamente uma versão aprovada e imutável; execuções guardam a versão exata utilizada.

O registro do modelo deve incluir checkpoint/hash do encoder, dimensão do embedding, contrato de bandas e normalização, sensores, política temporal, resolução, taxonomia, classes, região/bioma de validade, versões do dataset, splits, métricas, limiares, dependências e artefatos.

Detecte incompatibilidades antes da execução: um classificador treinado em embeddings anuais ou com sensores diferentes não deve ser aplicado silenciosamente aos embeddings recentes. Se não houver conjunto rotulado suficiente, entregue o fluxo de coleta e treinamento e exponha esse bloqueio com clareza.

## 9. Pipeline assíncrono e confiabilidade

Etapas: validar geometria/período; selecionar modelo compatível; consultar cenas; avaliar disponibilidade; materializar/preparar; gerar embeddings; classificar; avaliar qualidade; pós-processar opcionalmente; calcular estatísticas; publicar artefatos.

Use estados persistidos como queued, running, cancelling, cancelled, succeeded, partial, insufficient_data e failed. Separe status da execução, etapa e avaliação científica.

- Cada etapa deve ter entradas versionadas, saídas, logs, duração, tentativas e erro estruturado.
- Implemente idempotência, leases, heartbeat, timeout, retry com backoff para falhas transitórias e retomada após reinício.
- Não repita automaticamente erros determinísticos de formato, modelo ou geometria.
- Não mantenha transação longa aberta durante download ou inferência.
- Cancelamento deve interromper novas etapas e workers de forma cooperativa, com limpeza segura de temporários.
- Publicação atômica de artefatos completos: não marque sucesso com arquivo truncado ou inacessível.
- Cache por conteúdo e configuração: área/tile, intervalo absoluto, IDs/versões de cenas, sensores, receita, checkpoint e dimensão. Embeddings não dependem do classificador; resultados classificados também dependem dele e da taxonomia.
- Novas cenas disponibilizadas tardiamente podem invalidar um cache temporal; registre consulta/manifestações e implemente atualização explícita.
- Imponha limites de área, pixels, concorrência, download, armazenamento e GPU, configuráveis.

## 10. Persistência e contratos

Use migrations SQL e PostGIS com índices espaciais. Modele, no mínimo:

- users/workspaces/memberships; projects; areas e versões de geometria.
- classification_runs; jobs; job_steps; job_events.
- scene_assets; run_scene_assets; processing_recipes; artifacts.
- taxonomies; classes; taxonomy_mappings.
- sample_sets; samples; sample_revisions; dataset_versions.
- model_versions; evaluations; approvals.
- class_statistics; review_sessions; corrections; audit_events.

Use UUIDs, foreign keys, timestamps, checks, índices e escopo por workspace. Guarde geometrias em CRS documentado, com validação. JSONB é adequado para configuração flexível; relações críticas e estados precisam de estrutura e integridade. Guarde rasters e pesos fora do banco, referenciados por chave de objeto, checksum, tamanho, CRS, resolução e metadados.

API REST versionada, documentada em OpenAPI, incluindo contratos para:

- projetos e áreas;
- prévia de disponibilidade;
- criação/listagem/detalhe/cancelamento de execuções;
- eventos e progresso;
- estatísticas e artefatos;
- taxonomias, modelos e compatibilidade;
- amostras, treinamento, avaliações e aprovação;
- revisões e exportações.

Valide payloads em ambas as pontas e gere tipos TypeScript do contrato quando viável. Não distribua validações divergentes entre Next.js e Go.

## 11. Resultados e revisão

Produza COG/GeoTIFF georreferenciado para classes, qualidade e escores relevantes; legenda e metadados JSON; CSV de áreas; exportação vetorial opcional em GeoPackage/GeoJSON com limites de tamanho.

- Preserve o raster bruto; filtros espaciais e unidade mínima de mapeamento são opcionais, parametrizados e geram um derivado próprio.
- Recorte ao polígono exato. Defina e documente a regra de área em pixels de borda, inclusive cobertura fracionária quando exigida.
- A soma das áreas classificadas, não determinadas e sem dados deve corresponder à área analisada dentro da tolerância explícita.
- Não informe intervalos de confiança estatística de áreas sem desenho amostral e método apropriados.
- Exporte manifesto com período, cenas, datas, sensores, qualidade, receitas e versões do modelo/classificador/taxonomia.
- Correções manuais geram nova versão. Preserve previsão original, revisão, autor e justificativa; não transforme automaticamente toda correção em verdade de treinamento.

## 12. Segurança e operação

Implemente autenticação, perfis administrador/analista/leitor e autorização por workspace em API, SSE, tiles e downloads. Use cookies seguros e proteções correspondentes ao mecanismo de sessão; não armazene segredos no frontend.

Valide uploads, limites, CRS, geometrias e caminhos; previna SSRF em referências de ativos externos e não aceite URLs arbitrárias como fonte de processamento. Proteja subprocessos contra injeção de comandos. Não carregue modelos serializados não confiáveis enviados por usuários.

Use URLs temporárias ou mediação autenticada para artefatos. Não exponha buckets, configurações de provedores ou logs com credenciais. Registre ações importantes e planeje backup/restauração de PostgreSQL e objetos.

Entregue Docker Compose para desenvolvimento com web, API, orquestrador, worker científico, PostGIS, armazenamento local compatível com S3 e tiles. Separe configuração CPU/GPU quando necessário. Serviços devem ter healthchecks, logs estruturados e identificador de execução; documente métricas de fila, falhas, processamento e GPU.

## 13. Implementação por incrementos

1. Inspeção, decisões de arquitetura e prova técnica real do OlmoEarth recente.
2. Base do projeto, autenticação, PostGIS, armazenamento, mapa e cadastro de áreas.
3. Disponibilidade de cenas, aquisição, preparação e qualidade observacional.
4. Worker OlmoEarth e geração de embeddings com rastreabilidade.
5. Amostras, treinamento, avaliação e registro de classificador compatível.
6. Classificação completa, visualização, estatísticas e exportações.
7. Revisão manual, recuperação de jobs, testes e documentação operacional.

Priorize um caminho vertical pequeno e real antes de ampliar a interface. Não adie indefinidamente treinamento/validação: sem isso, não existe classificação operacional confiável. Modos demonstrativos usam fixtures explicitamente identificadas, não resultados apresentados como reais.

## 14. Critérios de aceite verificáveis

- Um usuário cria uma área, define janela recente, verifica disponibilidade, inicia job e acompanha sem manter requisição HTTP longa aberta.
- Uma execução de integração real gera embeddings com checkpoint e contrato verificados; testes unitários não dependem de downloads constantes.
- Havendo classificador aprovado e compatível, a execução produz raster alinhado, estatísticas coerentes e exportações legíveis em QGIS.
- Sem classificador, a interface explica o pré-requisito e permite criar amostras e treinar; não há mapa fictício.
- Não entram cenas após o fim do período; janelas ampliadas ou sensores reduzidos são visíveis e registrados.
- Nuvens e ausência de observações não são convertidas em solo, água ou outra classe por preenchimento inválido.
- A validação separa fazendas/blocos e períodos; rótulos históricos não são única evidência independente de acurácia.
- Falhas de provedor, memória insuficiente, reinício e cancelamento são tratados sem jobs eternamente em running.
- Um usuário não acessa área, job, tile ou arquivo de outro workspace.
- A execução é reprodutível a partir do manifesto, versões e artefatos retidos.

Teste especialmente: geometria inválida, polígono com buracos, máscara/bordas, períodos sem cenas, modalidade ausente, falta de modelo, incompatibilidade temporal, costuras entre tiles, conservação de áreas, isolamento de acesso e exportação. Inclua teste de integração com pequena fixture georreferenciada e um smoke test real opt-in, com custo e credenciais documentados.

## 15. Entrega esperada

Entregue código executável; migrations; contratos OpenAPI; configuração de desenvolvimento; .env.example sem segredos; scripts reais de aquisição, inferência, treinamento e avaliação; testes essenciais; README com comandos verificados; decisões arquiteturais; guia de amostragem; documentação científica e de limitações; instruções para GPU, backup e atualização de modelos.

No relatório final, descreva o que funciona, como foi verificado, o que depende de credenciais ou amostras e quais limitações permanecem. Diferencie implementação concluída de hipótese ainda não validada.

Comece agora pela inspeção do ambiente e prova técnica; depois avance até o fluxo funcional completo dentro dos recursos disponíveis.
