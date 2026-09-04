# ADR-004 — OpenRouter como provedor inicial de pesquisa/LLM

- **Status:** aceito com validação antes da implementação
- **Data:** 2026-09-03 (documentação oficial consultada nesta data)

## Contexto

O monitor precisa descobrir e estruturar conteúdo atual, trocar modelos conforme custo/capacidade e preservar rastreabilidade. API externa e modelos mudam; o domínio não pode depender deles.

## Decisão

Definir `ResearchProvider` no consumidor e implementar `OpenRouterProvider`. Modelos/base URL/engine ficam no ambiente. Structured outputs usarão JSON Schema estrito somente em modelos compatíveis. Pesquisa usará a server tool beta `openrouter:web_search`; plugin `web` e `:online` estão depreciados. Respostas e citações são validadas/registradas e nunca publicadas automaticamente.

Fontes oficiais: [Web Search Server Tool](https://openrouter.ai/docs/guides/features/server-tools/web-search), [Structured Outputs](https://openrouter.ai/docs/guides/features/structured-outputs), [Quickstart](https://openrouter.ai/docs/quickstart).

## Alternativas consideradas

- API direta de um modelo: menos camada, maior lock-in e menos flexibilidade.
- Motor de busca + LLM separados: mais controle, mais contratos/custos; permanece alternativa se server tool beta for instável.
- Pesquisa humana apenas: maior controle, baixa escala; continuará necessária na revisão.
- Plugin web legado: rejeitado por depreciação.

## Consequências positivas

Seleção flexível, API unificada, grounding/citações e schemas estruturados com adaptador substituível.

## Consequências negativas

Fornecedor adicional, preços/roteamento variáveis, server tool beta e combinações modelo+tool+schema que exigem teste.

## Riscos

Hallucinação/citação falsa, prompt injection, vazamento, mudança de API, indisponibilidade e custo. Schemas, proveniência, limites, mock/contract tests, human-in-loop e fail closed mitigam.

## Gatilhos de revisão

Mudança/saída do beta, incompatibilidade de structured output + search, política de dados inadequada, custo/qualidade/SLA insuficiente ou provedor alternativo superior. Revalidar docs imediatamente antes de codificar e em upgrades.

