# ADR-001 — Monólito modular em Go

- **Status:** aceito
- **Data:** 2026-09-03

## Contexto

O MVP será operado em VPS pequena por equipe reduzida, reúne site, painel, importação, monitor e exportação e precisa de baixo consumo e distribuição simples. Consistência editorial favorece transações locais; a escala ainda é desconhecida.

## Decisão

Um binário Go com subcomandos e módulos por capacidade. Uma implantação e fronteiras internas (`domain`, `editorial`, `store`, `research`, `monitoring`, `export`, `web`, `auth`, `observability`). Interfaces isolam adaptadores; não haverá chamada entre módulos via rede.

## Alternativas consideradas

- Microsserviços: isolamento independente, mas aumentam deploy, rede, consistência e observabilidade sem demanda.
- Framework full-stack dinâmico: alta produtividade, mas diverge da operação/binário definidos; não traz benefício decisivo.
- Funções serverless: escala automática, porém execução agendada, SQLite local e previsibilidade ficam piores.

## Consequências positivas

Deploy/backup/debug simples, transações diretas, baixo overhead e extração futura possível a partir de fronteiras reais.

## Consequências negativas

Falha afeta mais capacidades; deploy conjunto; disciplina modular é necessária; trabalho pesado precisa de limites para não disputar HTTP.

## Riscos

“Monólito” virar pacote acoplado, abstração excessiva ou monitor degradar consulta. Testes de dependência, casos de uso e isolamento de recursos mitigam.

## Gatilhos de revisão

Escala independente comprovada, equipes/ciclos distintos, requisitos de isolamento/segurança, jobs excedendo recursos ou disponibilidade impossível com um processo.

