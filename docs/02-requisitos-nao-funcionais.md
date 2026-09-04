# Requisitos não funcionais

## Baseline do MVP

- uma instância Go e SQLite em volume local;
- página funcional a partir de 320 px;
- nenhuma chamada de LLM no caminho de leitura pública;
- queries parametrizadas, templates escapados e segredos fora do Git;
- `/admin` protegido e mutações com proteção básica contra CSRF/origin;
- monitor com timeout, limite de custo, idempotência e lock contra sobreposição;
- conteúdo público sempre ligado a fonte e data disponíveis;
- verificação de fonte por GET limitado, com timeout, redirects controlados, bloqueio de redes privadas e limite de bytes; nenhum corpo é persistido;
- mudanças de moderação refletem imediatamente na consulta e nas métricas;
- banco em WAL, foreign keys e busy timeout;
- backup consistente antes de migrations/importações destrutivas e deploy relevante.

## Validação econômica

Não há meta de cobertura no MVP. Gate:

```bash
go fmt ./...
go vet ./...
go build ./...
```

Smoke test manual cobre banco, importação, monitoramento simulado/real limitado, verificação de acessibilidade, publicação/quarentena, moderação de claim/uso da fonte, métricas, páginas e exportação.

Sem meta de cobertura, Playwright ou suíte extensa. O MVP exige testes unitários table-driven para: matriz `published` versus `quarantined`; fingerprint rejeitado bloqueando republicação; e métricas ignorando `rejected`, `quarantined`, `archived` e claims `metric_eligible = false`. E2E não é requisito inicial.

## Futuro por gatilho

- painel com mais usuários: sessões fortes, MFA, RBAC, auditoria e testes de autorização;
- coleta/fetch próprio: hardening SSRF/download;
- maior volume: métricas/alertas, filas e testes de carga;
- incidentes/regressões: cobertura direcionada;
- múltiplas instâncias/escritores: PostgreSQL;
- maior relevância pública: políticas formais de retenção, snapshots, correção e revisão.
