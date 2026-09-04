# Roadmap

## Fase 0 — documentação

**Objetivo:** decisões suficientes para começar sem transformar riscos futuros em bloqueadores atuais.

**Conclusão:** visão, stack, modelo editorial, schema mínimo, escopo, backlog e ADRs alinhados ao MVP rápido.

## Fase 1 — esqueleto executável

- módulo Go e CLI;
- `serve` e `migrate`;
- SQLite/WAL;
- Docker Compose e Caddy;
- templates/base visual;
- configuração e build.

**Done:** aplicação sobe localmente, cria o banco e entrega uma home vazia.

## Fase 2 — dados e importação

- migrations do schema mínimo;
- importador XLSX com dry-run;
- normalização/deduplicação básica;
- carga da planilha;
- exportação/download.

**Done:** base é importada sem duplicar e pode ser consultada pelo código.

## Fase 3 — página pública

- home e resumo;
- listagem alfabética;
- busca, filtros e ordenação;
- cartões e detalhes;
- fontes e metodologia;
- mobile first e identidade inspirada nas cores do WhatsApp, sem copiar marca/interface.

**Done:** smoke test manual completo em celular e desktop.

## Fase 4 — deploy MVP

- Oracle VPS;
- Docker/Caddy/HTTPS;
- volume persistente;
- backup operacional simples;
- domínio e canal de correção.

**Done:** primeira versão pública, restaurável e com fontes navegáveis.

## Fase 5 — painel editorial

Auth, papéis, edição, revisão, histórico, contraditório e publicação versionada. Inicia quando atualização por CLI se tornar incômoda ou houver outro colaborador.

## Fase 6 — monitoramento

OpenRouter, web search, candidatos, deduplicação, custo, scheduler e revisão. Inicia após o fluxo editorial estar estável.

## Fase 7 — hardening e escala

Testes automatizados proporcionais, observabilidade, MFA, auditoria avançada, backup externo, políticas formais e eventual PostgreSQL. Prioridade definida por tráfego, incidentes, equipe e custo real.

