# Instruções para agentes — VorcaroZAP

## Leitura obrigatória

Antes de implementar, leia `README.md`, visão, arquitetura, modelo editorial, modelo de dados, roadmap e ADRs relacionados.

## Prioridade do MVP

1. página pública e métricas;
2. importação da base inicial;
3. OpenRouter e monitoramento automático;
4. validação/publicação automática controlada;
5. painel simples de pós-moderação;
6. deploy econômico em uma VPS.

Não retire OpenRouter, métricas ou painel do MVP. Não transforme painel em CMS genérico nem implemente RBAC/MFA/workflow multiusuário agora.

## Regras

- Monólito Go, SQLite, SSR e uma instância.
- Domínio não importa HTTP, SQLite, Excelize ou OpenRouter.
- Grau A–E pertence à alegação; relevância 1–5 pertence à entidade; confiança técnica pertence ao candidato/run.
- Métricas contam somente registros públicos ativos.
- Automação publica somente quando o gate estrutural em Go e o gate semântico por segunda avaliação passarem, e a política Go autorizar o grau; caso contrário, quarentena.
- A/B podem ser publicados automaticamente após os dois gates; C exige linguagem e limites explícitos; D/E vão para quarentena por padrão.
- A LLM recomenda; a política Go decide. Estados editoriais e estados operacionais de `monitoring_runs` são ciclos diferentes.
- Admin pode desaprovar, restaurar e revisar; toda moderação altera imediatamente a consulta e métricas.
- Queries parametrizadas, templates escapados, segredos fora do Git e proteção básica do `/admin` são obrigatórios.
- Não criar microsserviços, SPA, Redis ou PostgreSQL no MVP.
- Sem meta de cobertura. Testes table-driven de decisão de publicação, bloqueio por fingerprint rejeitado e métricas públicas são obrigatórios. Execute `go fmt`, `go vet`, `go build` e smoke checklist.

Mudanças de publicação automática, schema, stack ou fronteiras exigem ADR.
