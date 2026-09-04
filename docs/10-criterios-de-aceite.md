# Critérios de aceite

## Gate técnico econômico

```bash
go fmt ./...
go vet ./...
go build ./...
```

Sem percentual de cobertura ou E2E obrigatório no MVP.

## Smoke funcional

1. subir ambiente/banco vazio;
2. migrar e importar planilha sem duplicação;
3. consultar home, métricas, busca, filtros e detalhe;
4. executar monitor limitado e registrar custo;
5. candidato válido vira público;
6. candidato incompleto vai para quarentena;
7. desaprovar remove de página, busca, métricas e XLSX;
8. restaurar/autorizar reverte a visibilidade;
9. execuções simultâneas do monitor não se sobrepõem;
10. reiniciar containers preservando SQLite;
11. acessar `/admin` sem credencial falha;
12. baixar XLSX apenas com conteúdo público.

## Gate automático de publicação

- schema/citação/URL válidos;
- identidade sem ambiguidade relevante;
- alegação não excede a fonte;
- classificação A–E e linguagem compatíveis;
- trecho/localizador suficiente;
- ausência de PII desnecessária;
- fingerprint não rejeitado;
- custo dentro do limite.

## Não bloqueiam o MVP

MFA, RBAC multiusuário, dupla revisão, snapshots, trilha imutável, métricas históricas, cobertura abrangente, E2E, observabilidade completa, HA, PostgreSQL e política jurídica operacional extensa.

