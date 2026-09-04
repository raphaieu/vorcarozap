# Requisitos não funcionais

## Baseline obrigatório do MVP

- **Simplicidade:** uma aplicação Go, um banco SQLite e uma implantação.
- **Responsividade:** uso confortável a partir de 320 px e sem rolagem horizontal do conteúdo principal.
- **Desempenho:** páginas públicas não chamam serviços externos; consultas devem permanecer rápidas com a base inicial.
- **Portabilidade:** execução local e por Docker Compose.
- **Integridade:** foreign keys, migrations explícitas, queries parametrizadas e importação transacional.
- **Segurança básica:** HTTPS em produção, escape dos templates, segredos fora do Git, sem endpoints públicos de escrita e sem stack trace público.
- **Rastreabilidade editorial:** fonte e data disponíveis junto da associação publicada.
- **Operação:** banco em volume local persistente e procedimento simples de cópia/backup consistente antes de deploy ou importação relevante.

SQLite deve usar WAL, foreign keys, busy timeout e synchronous NORMAL conforme ADR-002.

## Validação econômica

Não existe meta de cobertura no primeiro lançamento. Antes de publicar:

```bash
go fmt ./...
go vet ./...
go build ./...
```

Executar smoke test manual de:

1. inicialização em banco vazio;
2. migration;
3. dry-run e importação;
4. home;
5. busca/filtro;
6. detalhe e abertura de fonte;
7. download da planilha;
8. restart preservando dados.

Testes automatizados entram por regressão ou quando lógica crítica justificar, sem percentual obrigatório.

## Requisitos futuros registrados

Quando houver painel, automação, múltiplos colaboradores ou tráfego relevante, implementar gradualmente:

- autenticação forte, MFA, RBAC, CSRF e rate limit;
- testes unitários de regras editoriais;
- integração para banco/importação/exportação;
- contratos do provedor de LLM;
- E2E das jornadas críticas;
- métricas, alertas, logs estruturados e budget;
- política formal de RPO/RTO, retenção e restauração;
- revisão WCAG completa e testes automatizados de acessibilidade;
- hardening de upload, fetch remoto, SSRF e prompt injection.

Os itens futuros tornam-se obrigatórios quando a funcionalidade ou risco correspondente for introduzido.

