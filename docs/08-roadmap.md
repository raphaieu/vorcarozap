# Roadmap

Cada fase termina com demonstração, testes, documentação e riscos atualizados. Datas dependem de equipe e validações; a ordem evita publicar antes de existir governança.

| Fase | Objetivo e entregas | Dependências | Riscos | Critério de conclusão |
|---|---|---|---|---|
| 0 — Fundação documental | visão, requisitos, editorial, arquitetura, dados, segurança, LLM, ADRs, riscos, backlog e aceite | briefing e inspeção do acervo | decisões abstratas ou inconsistentes | documentos revisados; pendências têm dono; nenhuma funcionalidade substancial antecipada |
| 1 — Fundação técnica | Go/CLI, config, Compose/Caddy, SQLite/WAL, goose/sqlc, health, logs, CI e testes | F0; versão estável/dependências verificadas | toolchain, permissões de volume, migration ruim | serviço sobe em ambiente limpo; migration e smoke test passam; shutdown/health/logs comprovados |
| 2 — Base editorial | entidades, relações, claims, evidências/fontes, contraditório, histórico e importador XLSX | F1; mapeamento editorial da planilha | identidade errada e conversão indevida da escala legada | dry-run relata 151 linhas sem publicar; aplicação transacional; invariantes/testes passam |
| 3 — Página pública | home, resumo, busca/filtros/ordenação, cartões, detalhe/fontes, responsividade e acessibilidade | F2; conteúdo aprovado fictício/teste | linguagem ambígua, performance e acessibilidade | jornadas públicas, WCAG e metas de desempenho aprovadas; nenhum rascunho exposto |
| 4 — Administração | login/MFA, RBAC, edição, revisão, merge, contraditório, auditoria | F2; política de papéis | tomada de conta e bypass de autorização | matriz de acesso testada; revisão/publicação e recuperação de sessão auditáveis |
| 5 — Monitoramento | ResearchProvider/OpenRouter, server web search, schemas, descoberta/verificação, dedupe, jobs internos, custo e revisão | F4; credencial/modelos/teto validados | beta/API, hallucination, prompt injection, custo | fixtures/falhas/idempotência passam; execução real controlada gera somente pending_review |
| 6 — Exportação/transparência | XLSX, metodologia, atualizações/correções, versão/hash | F2–F5 | divergência e vazamento de campos internos | arquivo nasce do snapshot DB, confere com visão pública e publicação é atômica |
| 7 — Produção | Oracle VPS, HTTPS, hardening, backup/restore, observabilidade, alertas e runbooks | todas; domínio/RPO/RTO/políticas | perda de dados, incidente e operação manual | restore ensaiado; TLS/headers/alertas verificados; checklist editorial e go-live assinados |

## Marcos

- **M0:** fundação validada (resultado desta etapa após revisão humana).
- **M1:** esqueleto executável e banco migrável.
- **M2:** 151 linhas importáveis como candidatos com relatório e sem publicação.
- **M3:** consulta pública completa sobre dados fictícios/aprovados.
- **M4:** fluxo humano seguro de ponta a ponta.
- **M5:** monitor controlado e auditável.
- **M6:** release público restaurável.

Não iniciar ingestão real automatizada antes de M4. Não fazer go-live antes da validação editorial/jurídica e do teste de restauração.

