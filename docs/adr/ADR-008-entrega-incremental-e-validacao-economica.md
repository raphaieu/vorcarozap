# ADR-008 — Entrega incremental e validação econômica

- **Status:** aceito
- **Data:** 2026-09-04

## Contexto

O desenho completo inclui segurança avançada, painel, automação, testes e operação robusta. Implementar tudo antes do primeiro acesso aumentaria tempo e consumo de tokens sem evidência de demanda.

## Decisão

Entregar primeiro uma aplicação pública somente de leitura, alimentada por CLI, com controles básicos de baixo custo. Não haverá meta de cobertura nem E2E no MVP; build, análise estática e smoke test manual formam o gate inicial.

Funcionalidades futuras permanecem documentadas e sobem de prioridade por gatilhos observáveis: tráfego, colaboração, regressão, incidente, frequência de atualização ou integração externa.

## Alternativas

- Implementar o desenho completo: maior robustez inicial, maior prazo e custo.
- Protótipo sem banco/arquitetura: lançamento ainda mais rápido, mas cria retrabalho na importação e evolução.

## Consequências

Menor prazo, custo e superfície de ataque inicial. Há maior dependência de verificação manual e menor proteção contra regressões, aceita conscientemente até surgirem gatilhos.

## Gatilhos de revisão

Erros recorrentes, colaboradores adicionais, painel, OpenRouter, uploads, tráfego relevante, contestação ou necessidade de releases frequentes.

