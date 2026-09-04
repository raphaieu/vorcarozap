# Critérios de aceite

## Gate técnico econômico

```bash
go fmt ./...
go vet ./...
go build ./...
```

Sem percentual de cobertura ou E2E obrigatório no MVP.

São obrigatórios testes unitários table-driven para decisão `published`/`quarantined`, bloqueio de republicação por fingerprint rejeitado e exclusão de estados não públicos nas métricas.

## Smoke funcional

1. subir ambiente/banco vazio;
2. migrar e importar planilha sem duplicação, publicando apenas linhas `curated_seed` elegíveis e colocando as demais em quarentena;
3. consultar home, métricas, busca, filtros e detalhe;
4. executar monitor limitado e registrar custo;
5. candidato A/B válido vira público, C só com linguagem/limites explícitos e D/E vai para quarentena;
6. candidato incompleto vai para quarentena;
7. desaprovar remove de página, busca, métricas e XLSX;
8. restaurar/autorizar reverte a visibilidade;
9. execuções simultâneas do monitor não se sobrepõem;
10. reiniciar containers preservando SQLite;
11. acessar `/admin` sem credencial falha;
12. baixar XLSX apenas com conteúdo público.

## Gates automáticos de publicação

- Gate estrutural Go aprova schema, URL, metadados, trecho/localizador, datas, grau, vínculo, PII, tamanho, dedupe, fingerprint e orçamento.
- Gate semântico estruturado aprova identidade, suporte, não extrapolação, atribuição, grau, ausência de inferência ilícita e ambiguidades vazias.
- Política Go publica A/B aprovados e C aprovado com linguagem/limites explícitos; D/E ficam em quarentena.
- Homônimo, fonte inacessível, acusação criminal não confirmada, divergência entre estágios ou rejeição semelhante sempre levam a quarentena.

## Não bloqueiam o MVP

MFA, RBAC multiusuário, dupla revisão, snapshots, trilha imutável, métricas históricas, cobertura abrangente, E2E, observabilidade completa, HA, PostgreSQL e política jurídica operacional extensa.
