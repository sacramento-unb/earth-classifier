# decisões de arquitetura

O caminho de negócio é Go + PostgreSQL/PostGIS. A fila usa `jobs`/`job_steps` com `FOR UPDATE SKIP LOCKED`, lease de dez minutos, tentativas e estados persistidos. A API não trafega tensores; artefatos devem ficar em storage S3/MinIO e o banco guarda checksum, CRS, resolução e manifesto.

O serviço `orchestrator` consulta o STAC, registra cenas e interrompe honestamente quando não existe modelo aprovado. O Python fica em `workers/geo_ml`, sem API de negócio própria: aquisição/preparação, OlmoEarth, treinamento e aplicação são scripts/worker científico. O Compose reserva TiTiler para tiles COG.

As áreas são armazenadas como MultiPolygon EPSG:4326 e a área em hectares é calculada com `ST_Transform(...,6933)`. Toda consulta de entidade inclui `workspace_id`; o header de workspace é um mecanismo local de desenvolvimento, não autenticação de produção.

Estados técnicos são separados de classes temáticas: `insufficient_data`, `undetermined` e `nodata` não são convertidos em solo exposto, água ou qualquer outra classe. O status de uma execução pode ser `insufficient_data` mesmo quando o catálogo tem cenas: falta de observação por pixel e falta de classificador são motivos diferentes no manifesto/eventos.
