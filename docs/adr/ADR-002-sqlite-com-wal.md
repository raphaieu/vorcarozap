# ADR-002 — SQLite com WAL

- **Status:** aceito
- **Data:** 2026-09-03

## Contexto

Há uma instância, poucos escritores e prioridade por operação simples. O domínio é relacional e requer constraints, transações, auditoria e exportação consistente.

## Decisão

SQLite em volume persistente local com `database/sql`, sqlc e goose. Verificar por conexão: `journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=5000`, `synchronous=NORMAL`. Escritas pequenas, monitor exclusivo e backup online consistente; nunca NFS/object storage nem cópia crua durante escrita.

## Alternativas consideradas

- PostgreSQL desde o início: concorrência/HA superiores, com maior operação sem necessidade atual.
- Banco documental: flexível, mas enfraquece integridade relacional e consultas do produto.
- Arquivos/planilha: simples, sem concorrência, constraints e histórico confiáveis.

## Consequências positivas

Pouca infraestrutura, arquivo portátil, desempenho local e transações fortes para escala inicial.

## Consequências negativas

Um escritor por vez, HA/replicação limitadas, cuidado com WAL/backup e algumas migrations mais trabalhosas.

## Riscos

Contenção, disco cheio/corrupto, configuração por conexão inconsistente e falsa sensação de que volume remoto é seguro. Métricas, limites, integridade e restore mitigam.

## Gatilhos de revisão

Múltiplas réplicas ou escritores, workers remotos, `SQLITE_BUSY`/latência relevantes, analytics pesado, HA, crescimento operacional incompatível ou restore/RPO não atendidos. Destino provável: PostgreSQL.

