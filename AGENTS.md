# Instruções para agentes — VorcaroZAP

## Antes de alterar código

1. Leia `README.md`, `docs/00-visao-do-produto.md`, `docs/08-roadmap.md` e os ADRs relacionados.
2. Preserve o objetivo do MVP rápido: página pública de leitura, importação por CLI e SQLite.
3. Não antecipe painel, autenticação, OpenRouter, scheduler, snapshots, observabilidade avançada ou suíte E2E.
4. Não transforme itens FUTURO em bloqueadores sem registrar um gatilho concreto.

## Regras de implementação

- Prefira stdlib e dependências já decididas.
- Não crie microsserviços, SPA, Redis ou PostgreSQL no MVP.
- Não acople domínio a HTTP, SQLite, Excelize ou futuro provedor de LLM.
- Use queries parametrizadas e templates com escape.
- Não faça commit de chaves, `.env`, bancos SQLite ou arquivos temporários.
- Preserve fontes, datas e classificação editorial importadas.
- Não converta automaticamente a classificação legada da planilha em verdade editorial.
- Não publique conteúdo produzido por IA sem revisão humana quando essa capacidade existir.

## Validação do MVP

- Execute `go fmt ./...`, `go vet ./...` e `go build ./...`.
- Execute o smoke test manual descrito nos critérios de aceite.
- Não busque cobertura de testes por percentual nesta fase.
- Adicione teste automatizado apenas quando ele proteger lógica crítica já implementada ou reproduzir regressão real.

## Alterações arquiteturais

Mudanças em banco, stack, publicação automática ou fronteiras do monólito exigem ADR novo ou atualização explícita de ADR existente.

