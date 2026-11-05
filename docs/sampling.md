# guia de amostragem e aprovação

1. Crie um `sample_set` com classe, geometria, fonte, autor, período da observação e revisão. Use áreas de referência e revisão humana; MapBiomas é apoio histórico, não verdade independente automática.
2. Cubra vegetação nativa, pastagem, agricultura, silvicultura, solo exposto, água e construído em biomas, estações e situações difíceis. Inclua explicitamente lavoura colhida, pastagem seca e formação campestre nativa.
3. Gere embeddings com a mesma janela, sensores, receita, resolução e checkpoint que serão usados na operação. Não misture embeddings anuais com janelas recentes.
4. Separe por fazenda/bloco espacial e período temporal. Pixels do mesmo talhão ou janelas sobrepostas não podem cair em splits independentes.
5. Compare uma referência de bandas/índices/estatísticas temporais com embeddings + Random Forest ou regressão logística. Registre matriz de confusão, precision, recall, F1 e IoU por classe, macro-F1, cobertura, abstenção e erros por região/estação.
6. Calibre probabilidades em conjunto separado quando necessário. A interface chama o valor de score; não chama de acurácia.
7. Só altere `model.json.status` para `approved` após revisão da região/bioma, período, sensores, classes e métricas. A promoção deve gerar auditoria e manter o artefato imutável.

Não promova uma categoria parental automaticamente quando uma subclasse não tem suporte. Em dúvida, use `undetermined`.
