# Backlog inicial priorizado

## P0 — colocar no ar

| ID | Entrega | Aceite enxuto |
|---|---|---|
| VZ-001 | Projeto Go e CLI | `serve` e `migrate` compilam e exibem erros úteis |
| VZ-002 | SQLite e migrations | banco vazio é criado com PRAGMAs esperados |
| VZ-003 | Docker/Caddy | aplicação sobe via Compose e persiste `/data` |
| VZ-004 | Layout base | página mobile first usa identidade própria verde e disclaimer |
| VZ-005 | Schema editorial mínimo | entidades, relações, claims e fontes são consultáveis |
| VZ-006 | Dry-run XLSX | mapeamento e erros são exibidos sem gravar |
| VZ-007 | Importação XLSX | carga transacional e repetição não duplicam o arquivo |
| VZ-008 | Listagem | nomes aparecem em ordem alfabética com informações principais |
| VZ-009 | Busca e filtros | nome, categoria, grau e relevância funcionam |
| VZ-010 | Detalhe e fontes | associação, limites, classificação, fonte e data aparecem |
| VZ-011 | Download | XLSX disponível e identificado pela data da base |
| VZ-012 | Deploy | HTTPS, volume e restart validados na Oracle VPS |
| VZ-013 | Smoke checklist | build e oito jornadas manuais documentadas passam |

## P1 — facilitar atualização

- painel administrativo;
- autenticação e papéis;
- edição, revisão e publicação;
- histórico/correções estruturados;
- contraditório;
- resumo versionado;
- backup externo e restauração automatizada.

## P2 — automatizar pesquisa

- OpenRouter/ResearchProvider;
- descoberta e verificação;
- candidatos e deduplicação;
- scheduler;
- orçamento e logs;
- proveniência detalhada;
- snapshots quando necessários.

## P3 — endurecer e escalar

- cobertura automatizada crescente;
- E2E;
- MFA e políticas avançadas;
- observabilidade/alertas;
- API pública;
- PostgreSQL/múltiplas instâncias se métricas exigirem.

Uma tarefa futura sobe de prioridade quando a funcionalidade correspondente começar, surgir regressão/risco concreto ou o uso real justificar o investimento.

